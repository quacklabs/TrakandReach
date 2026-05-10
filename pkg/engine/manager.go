package engine

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/playwright-community/playwright-go"
	"github.com/username/trakand-reach/pkg/bridge"
	"github.com/username/trakand-reach/pkg/db"
	"github.com/username/trakand-reach/pkg/filter"
	"github.com/username/trakand-reach/pkg/models"
)

type Event struct {
	SessionID string
	Type      string
	Data      interface{}
}

type SessionInstance struct {
	Model       *models.Session
	Context     playwright.BrowserContext
	Page        playwright.Page
	IsAlive     bool
	Events      chan Event
	StopCapture context.CancelFunc
	Ready       bool
	LastQR      string
}

type Manager struct {
	pw               *playwright.Playwright
	browser          playwright.Browser
	sessions         sync.Map // string -> *SessionInstance
	repo             *db.Repository
	baseDir          string
	mu               sync.Mutex
	isStarted        bool
	warmedBrowsers   map[string]bool
	GlobalWebhookURL string
	WebhookSecret    string
	Filter           *filter.SpamFilter
}

func NewManager(repo *db.Repository) (*Manager, error) {
	home, _ := os.UserHomeDir()
	baseDir := filepath.Join(home, ".trakand_reach")
	os.MkdirAll(filepath.Join(baseDir, "browserSessions"), 0755)

	// Initialize Spam Filter
	spamFile := "spam.txt"
	if _, err := os.Stat(spamFile); os.IsNotExist(err) {
		// Try in the same directory as the executable if not in CWD
		execPath, err := os.Executable()
		if err == nil {
			spamFile = filepath.Join(filepath.Dir(execPath), "spam.txt")
		}
	}

	return &Manager{
		repo:           repo,
		baseDir:        baseDir,
		warmedBrowsers: make(map[string]bool),
		Filter:         filter.NewSpamFilter(spamFile),
	}, nil
}

func (m *Manager) SetWebhookConfig(url, secret string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.GlobalWebhookURL = url
	m.WebhookSecret = secret
}

func (m *Manager) Start() error {
	m.mu.Lock()
	if m.isStarted {
		m.mu.Unlock()
		return nil
	}

	log.Printf("Starting Trakand Reach Engine...")

	pw, err := playwright.Run()
	if err != nil {
		return fmt.Errorf("could not start playwright: %v", err)
	}
	m.pw = pw
	m.isStarted = true
	m.mu.Unlock()

	// Migration from legacy JSON
	legacyJSON := filepath.Join(m.baseDir, "sessions.json")
	if err := m.repo.ImportFromJSON(legacyJSON); err != nil {
		log.Printf("Warning: legacy JSON migration failed: %v", err)
	}

	// Warming
	go m.WarmBrowser("webkit")

	// Auto-resume sessions
	go m.resumeSessions()

	// Ensure default session
	go m.ensureDefaultSession()

	return nil
}

func (m *Manager) ensureDefaultSession() {
	// Give it a moment for the engine to stabilize
	time.Sleep(2 * time.Second)

	sessionID := "default"
	if _, ok := m.sessions.Load(sessionID); ok {
		return
	}

	// Check DB
	var session *models.Session
	sessions, err := m.repo.GetSessions()
	if err == nil {
		for _, s := range sessions {
			if s.ID == sessionID {
				session = s
				break
			}
		}
	}

	if session == nil {
		session = &models.Session{
			ID: sessionID,
			DeviceInfo: models.DeviceInfo{
				UserAgent:  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15",
				Width:      1280,
				Height:     720,
				PixelRatio: 1.0,
			},
			LastURL: "https://web.whatsapp.com",
		}
		m.repo.SaveSession(session)
	}

	log.Printf("Starting default WhatsApp session...")
	_, err = m.StartSession(session)
	if err != nil {
		log.Printf("Failed to start default session: %v", err)
	}
}

func (m *Manager) WarmBrowser(browserType string) {
	log.Printf("Warming %s browser...", browserType)
	var bt playwright.BrowserType
	if browserType == "chromium" {
		bt = m.pw.Chromium
	} else if browserType == "firefox" {
		bt = m.pw.Firefox
	} else {
		bt = m.pw.WebKit
	}

	browser, err := bt.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true),
	})
	if err != nil {
		log.Printf("Failed to warm browser %s: %v", browserType, err)
		return
	}
	browser.Close()
	m.mu.Lock()
	m.warmedBrowsers[browserType] = true
	m.mu.Unlock()
	log.Printf("%s browser warmed ✅", browserType)
}

func (m *Manager) GetHealth() map[string]interface{} {
	activeCount := 0
	allReady := true
	m.sessions.Range(func(_, value interface{}) bool {
		activeCount++
		inst := value.(*SessionInstance)
		if !inst.Ready {
			allReady = false
		}
		return true
	})

	m.mu.Lock()
	warmed := m.warmedBrowsers["webkit"]
	m.mu.Unlock()

	return map[string]interface{}{
		"engine_running":  m.isStarted,
		"sessions_active": activeCount,
		"webkit_warmed":   warmed,
		"loop_ready":      allReady && m.isStarted,
		"db_status":       "connected",
	}
}

func (m *Manager) GetSessionsStatus() []map[string]interface{} {
	var results []map[string]interface{}
	m.sessions.Range(func(key, value interface{}) bool {
		id := key.(string)
		inst := value.(*SessionInstance)
		results = append(results, map[string]interface{}{
			"id":               id,
			"ready":            inst.Ready,
			"connection_state": inst.Model.ConnectionState,
			"owner_jid":        inst.Model.OwnerJID,
		})
		return true
	})
	return results
}

func (m *Manager) GetSessionStatus(id string) (map[string]interface{}, error) {
	val, ok := m.sessions.Load(id)
	if !ok {
		return nil, fmt.Errorf("session %s not found", id)
	}
	inst := val.(*SessionInstance)
	return map[string]interface{}{
		"id":               id,
		"ready":            inst.Ready,
		"qr":               inst.LastQR,
		"connection_state": inst.Model.ConnectionState,
		"owner_jid":        inst.Model.OwnerJID,
		"profile_name":     inst.Model.ProfileName,
	}, nil
}

func (m *Manager) resumeSessions() {
	sessions, err := m.repo.GetSessions()
	if err != nil {
		log.Printf("Failed to load sessions for resume: %v", err)
		return
	}

	for _, s := range sessions {
		if s.LastURL != "" {
			log.Printf("Auto-resuming session: %s", s.ID)
			go m.StartSession(s)
		}
	}
}

func (m *Manager) StartSession(s *models.Session) (*SessionInstance, error) {
	if val, ok := m.sessions.Load(s.ID); ok {
		return val.(*SessionInstance), nil
	}

	userDataDir := filepath.Join(m.baseDir, "browserSessions", s.ID)
	os.MkdirAll(userDataDir, 0755)

	options := playwright.BrowserTypeLaunchPersistentContextOptions{
		Headless: playwright.Bool(true),
		UserAgent: playwright.String(s.DeviceInfo.UserAgent),
		Viewport: &playwright.Size{
			Width:  s.DeviceInfo.Width,
			Height: s.DeviceInfo.Height,
		},
		DeviceScaleFactor: playwright.Float(s.DeviceInfo.PixelRatio),
		IsMobile:          playwright.Bool(s.DeviceInfo.Device.Type != "desktop"),
		HasTouch:          playwright.Bool(s.DeviceInfo.Device.Type != "desktop"),
		Locale:            playwright.String(s.DeviceInfo.Language),
		BypassCSP:         playwright.Bool(true),
		IgnoreHttpsErrors: playwright.Bool(true),
	}

	context, err := m.pw.WebKit.LaunchPersistentContext(userDataDir, options)
	if err != nil {
		return nil, err
	}

	page, err := context.NewPage()
	if err != nil {
		context.Close()
		return nil, err
	}

	inst := &SessionInstance{
		Model:   s,
		Context: context,
		Page:    page,
		IsAlive: true,
		Events:  make(chan Event, 100),
		Ready:   false,
	}

	// Expose Function for Bridge
	err = page.ExposeFunction("trakand_emit", func(args ...interface{}) interface{} {
		if len(args) < 2 {
			return nil
		}
		name, _ := args[0].(string)
		data := args[1]

		// Filter Spam
		if name == "message_new" {
			if mData, ok := data.(map[string]interface{}); ok {
				from, _ := mData["from"].(string)
				body, _ := mData["body"].(string)
				if m.Filter != nil && m.Filter.IsSpam(from, body) {
					log.Printf("[Session: %s] Discarding spam message from %s", s.ID, from)
					return nil
				}
			}
		}

		// Inject session_id if data is a map
		if mData, ok := data.(map[string]interface{}); ok {
			mData["session_id"] = s.ID
		}

		// Capture QR
		if name == "qr" {
			if qr, ok := data.(string); ok {
				inst.LastQR = qr
			}
		}

		ev := Event{
			SessionID: s.ID,
			Type:      name,
			Data:      data,
		}

		inst.Events <- ev

		// Integrated Webhook Dispatch
		webhookURL := s.WebhookURL
		if webhookURL == "" {
			m.mu.Lock()
			webhookURL = m.GlobalWebhookURL
			m.mu.Unlock()
		}

		if webhookURL != "" {
			go m.dispatchWebhook(webhookURL, ev)
		}

		// Update DB on state changes
		if name == "connection_update" {
			m.updateSessionState(s.ID, data)
		}
		return nil
	})
	if err != nil {
		log.Printf("Failed to expose function: %v", err)
	}

	// Inject Bridge
	err = context.AddInitScript(playwright.Script{
		Content: playwright.String(bridge.JSBridge),
	})
	if err != nil {
		log.Printf("Failed to add init script: %v", err)
	}

	if s.LastURL != "" {
		_, err = page.Goto(s.LastURL, playwright.PageGotoOptions{
			WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		})
		if err != nil {
			log.Printf("Failed to navigate to last URL: %v", err)
		}
	}

	inst.Ready = true
	m.sessions.Store(s.ID, inst)
	return inst, nil
}

func (m *Manager) updateSessionState(id string, data interface{}) {
	d, ok := data.(map[string]interface{})
	if !ok {
		return
	}

	sessions, _ := m.repo.GetSessions()
	for _, s := range sessions {
		if s.ID == id {
			if state, ok := d["state"].(string); ok {
				s.ConnectionState = state
			}
			if jid, ok := d["owner_jid"].(string); ok {
				s.OwnerJID = jid
			}
			if name, ok := d["profile_name"].(string); ok {
				s.ProfileName = name
			}
			m.repo.SaveSession(s)
			break
		}
	}
}

func (m *Manager) SendMessage(sessionID, to, text string) error {
	val, ok := m.sessions.Load(sessionID)
	if !ok {
		return fmt.Errorf("session %s not found", sessionID)
	}
	inst := val.(*SessionInstance)

	// Try Internal Store first
	result, err := inst.Page.Evaluate(`async (to, text) => {
		if (window.trakand_bridge && window.trakand_bridge.sendMessage) {
			return await window.trakand_bridge.sendMessage(to, text);
		}
		return { success: false, error: 'Bridge not ready' };
	}`, to, text)

	if err == nil {
		res := result.(map[string]interface{})
		if success, _ := res["success"].(bool); success {
			return nil
		}
	}

	// Fallback: URL Navigation (Precision)
	isNumeric := true
	for _, c := range to {
		if c < '0' || c > '9' {
			isNumeric = false
			break
		}
	}

	if isNumeric && len(to) >= 10 {
		url := fmt.Sprintf("https://web.whatsapp.com/send?phone=%s", to)
		_, err = inst.Page.Goto(url, playwright.PageGotoOptions{
			WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		})
		if err != nil {
			return err
		}
		// Basic UI Fallback: Type and Enter
		err = inst.Page.Keyboard().Type(text)
		if err != nil {
			return err
		}
		err = inst.Page.Keyboard().Press("Enter")
		return err
	}

	return fmt.Errorf("could not send message: %v", err)
}

func (m *Manager) dispatchWebhook(url string, ev Event) {
	payload := map[string]interface{}{
		"account": ev.SessionID,
		"event":   ev.Type,
		"data":    ev.Data,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Webhook marshal error: %v", err)
		return
	}

	// Simple retry logic
	for i := 0; i < 3; i++ {
		req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
		if err != nil {
			log.Printf("Webhook request creation error: %v", err)
			return
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "TrakandReach-Go/1.0")

		m.mu.Lock()
		secret := m.WebhookSecret
		m.mu.Unlock()

		if secret != "" {
			h := hmac.New(sha256.New, []byte(secret))
			h.Write(body)
			signature := hex.EncodeToString(h.Sum(nil))
			req.Header.Set("X-Trakand-Signature", signature)
		}

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode < 300 {
			resp.Body.Close()
			return
		}

		if err == nil {
			resp.Body.Close()
		}

		time.Sleep(time.Duration(i+1) * time.Second)
	}
}

func (m *Manager) Stop() {
	m.sessions.Range(func(key, value interface{}) bool {
		inst := value.(*SessionInstance)
		inst.Page.Close()
		inst.Context.Close()
		return true
	})
	if m.pw != nil {
		m.pw.Stop()
	}
}
