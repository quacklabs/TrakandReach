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
	s.mux.HandleFunc("/reach/sessions", s.handleSessions)
	s.mux.HandleFunc("/reach/sessions/", s.handleSessionDetail)
	s.mux.HandleFunc("/reach/send", s.handleSendMessage)
	s.mux.HandleFunc("/ws", s.handleWebSocket)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	health := s.manager.GetHealth()
	w.Header().Set("Content-Type", "application/json")
	// If the manager is not yet fully started but the server is up, we should still return 200 with status
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(health)
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		sessions := s.manager.GetSessionsStatus()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sessions)
		return
	}

	if r.Method == http.MethodPost {
		var req models.Session
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if req.ID == "" {
			http.Error(w, "session_id is required", http.StatusBadRequest)
			return
		}

		// Default values if not provided
		if req.DeviceInfo.UserAgent == "" {
			req.DeviceInfo.UserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15"
		}
		if req.DeviceInfo.Width == 0 {
			req.DeviceInfo.Width = 1280
		}
		if req.DeviceInfo.Height == 0 {
			req.DeviceInfo.Height = 720
		}
		if req.LastURL == "" {
			req.LastURL = "https://web.whatsapp.com"
		}

		inst, err := s.manager.StartSession(&req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":     "starting",
			"session_id": inst.Model.ID,
		})
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func (s *Server) handleSessionDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Path is /reach/sessions/{id} or /reach/sessions/{id}/qr
	path := r.URL.Path
	parts := bytes.Split([]byte(path), []byte("/"))
	if len(parts) < 4 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	sessionID := string(parts[3])
	status, err := s.manager.GetSessionStatus(sessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	if len(parts) == 5 && string(parts[4]) == "qr" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"session_id": sessionID,
			"qr":         status["qr"],
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (s *Server) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		SessionID string `json:"session_id"`
		To        string `json:"to"`
		Text      string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.SessionID == "" || req.To == "" || req.Text == "" {
		http.Error(w, "session_id, to, and text are required", http.StatusBadRequest)
		return
	}

	if err := s.manager.SendMessage(req.SessionID, req.To, req.Text); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
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
				"type": "event",
				"name": ev.Type,
				"data": ev.Data,
			})
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

