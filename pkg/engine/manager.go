package engine

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/playwright-community/playwright-go"
	"github.com/username/trakand-reach/pkg/bridge"
	"github.com/username/trakand-reach/pkg/db"
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
}

type Manager struct {
	pw       *playwright.Playwright
	browser  playwright.Browser
	sessions sync.Map // string -> *SessionInstance
	repo     *db.Repository
	baseDir  string
	mu       sync.Mutex
	isStarted bool
	warmedBrowsers map[string]bool
}

func NewManager(repo *db.Repository) (*Manager, error) {
	home, _ := os.UserHomeDir()
	baseDir := filepath.Join(home, ".trakand_reach")
	os.MkdirAll(filepath.Join(baseDir, "browserSessions"), 0755)

	return &Manager{
		repo:           repo,
		baseDir:        baseDir,
		warmedBrowsers: make(map[string]bool),
	}, nil
}

func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.isStarted {
		return nil
	}

	pw, err := playwright.Run()
	if err != nil {
		return fmt.Errorf("could not start playwright: %v", err)
	}
	m.pw = pw
	m.isStarted = true

	// Migration from legacy JSON
	legacyJSON := filepath.Join(m.baseDir, "sessions.json")
	if err := m.repo.ImportFromJSON(legacyJSON); err != nil {
		log.Printf("Warning: legacy JSON migration failed: %v", err)
	}

	// Warming
	go m.WarmBrowser("webkit")

	// Auto-resume sessions
	go m.resumeSessions()

	return nil
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
	m.sessions.Range(func(_, _ interface{}) bool {
		activeCount++
		return true
	})

	m.mu.Lock()
	warmed := m.warmedBrowsers["webkit"]
	m.mu.Unlock()

	return map[string]interface{}{
		"engine_running":  m.isStarted,
		"sessions_active": activeCount,
		"webkit_warmed":   warmed,
		"db_status":       "connected",
	}
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
	}

	// Expose Function for Bridge
	err = page.ExposeFunction("trakand_emit", func(args ...interface{}) interface{} {
		if len(args) < 2 {
			return nil
		}
		name, _ := args[0].(string)
		data := args[1]
		inst.Events <- Event{
			SessionID: s.ID,
			Type:      name,
			Data:      data,
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

func (m *Manager) GetSession(id string) (*SessionInstance, bool) {
	val, ok := m.sessions.Load(id)
	if !ok {
		return nil, false
	}
	return val.(*SessionInstance), true
}

func (m *Manager) GetPersistentSessions() ([]*models.Session, error) {
	return m.repo.GetSessions()
}

func (m *Manager) StopSession(id string) {
	val, ok := m.sessions.Load(id)
	if !ok {
		return
	}
	inst := val.(*SessionInstance)
	inst.Page.Close()
	inst.Context.Close()
	m.sessions.Delete(id)
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
