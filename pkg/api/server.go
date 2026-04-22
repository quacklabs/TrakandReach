package api

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/playwright-community/playwright-go"
	"github.com/username/trakand-reach/pkg/engine"
	"github.com/username/trakand-reach/pkg/img"
	"github.com/username/trakand-reach/pkg/models"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type Server struct {
	manager *engine.Manager
	mux     *http.ServeMux
}

func NewServer(manager *engine.Manager) *Server {
	s := &Server{
		manager: manager,
		mux:     http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("/reach/health", s.handleHealth)
	s.mux.HandleFunc("/reach/sessions", s.handleListSessions)
	s.mux.HandleFunc("/reach/send", s.handleSendMessage)
	s.mux.HandleFunc("/ws", s.handleWebSocket)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	health := s.manager.GetHealth()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health)
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	// Implementation deferred to manager/repo lookup
}

func (s *Server) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID string `json:"session_id"`
		To        string `json:"to"`
		Text      string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.manager.SendMessage(req.SessionID, req.To, req.Text); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "sent"})
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WS upgrade error: %v", err)
		return
	}
	defer conn.Close()

	// Initial Handshake
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return
	}

	var init struct {
		SessionID string            `json:"session_id"`
		Device    models.DeviceInfo `json:"deviceInfo"`
	}
	if err := json.Unmarshal(msg, &init); err != nil {
		return
	}

	session := &models.Session{
		ID:         init.SessionID,
		DeviceInfo: init.Device,
		AccessKey:  "default-key",
	}

	inst, err := s.manager.StartSession(session)
	if err != nil {
		log.Printf("Failed to start session: %v", err)
		return
	}

	// Forward Events & Stream Screenshots
	stop := make(chan bool)
	go s.streamScreenshots(conn, inst, stop)

	for {
		select {
		case ev := <-inst.Events:
			conn.WriteJSON(map[string]interface{}{
				"type":    "event",
				"account": ev.SessionID,
				"name":    ev.Type,
				"data":    ev.Data,
			})
			// Forward to Webhook if configured
			if inst.Model.WebhookURL != "" {
				go s.forwardWebhook(inst.Model.WebhookURL, ev)
			}
		}
	}
}

func (s *Server) streamScreenshots(conn *websocket.Conn, inst *engine.SessionInstance, stop chan bool) {
	ticker := time.NewTicker(800 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			screenshot, err := inst.Page.Screenshot(playwright.PageScreenshotOptions{
				Type:    playwright.ScreenshotTypeJpeg,
				Quality: playwright.Int(80),
			})
			if err != nil {
				continue
			}

			webpData, err := img.ToWebP(screenshot, 75)
			if err != nil {
				continue
			}

			// Binary Protocol: [6 Magic][2 Version][2 Type][Payload]
			var buf bytes.Buffer
			buf.WriteString("WREACH")
			binary.Write(&buf, binary.BigEndian, uint16(1)) // Version
			binary.Write(&buf, binary.BigEndian, uint16(1)) // Type: WebP
			buf.Write(webpData)

			if err := conn.WriteMessage(websocket.BinaryMessage, buf.Bytes()); err != nil {
				return
			}
		case <-stop:
			return
		}
	}
}

func (s *Server) forwardWebhook(url string, ev engine.Event) {
	payload, _ := json.Marshal(map[string]interface{}{
		"account": ev.SessionID,
		"event":   ev.Type,
		"data":    ev.Data,
	})

	// Simple retry logic
	for i := 0; i < 3; i++ {
		resp, err := http.Post(url, "application/json", bytes.NewBuffer(payload))
		if err == nil && resp.StatusCode < 300 {
			resp.Body.Close()
			return
		}
		time.Sleep(time.Duration(i+1) * time.Second)
	}
}
