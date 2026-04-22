package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/username/trakand-reach/pkg/db"
	"github.com/username/trakand-reach/pkg/engine"
)

func TestAPIRoutes(t *testing.T) {
	dbPath := "./test_api.db"
	defer os.Remove(dbPath)
	repo, _ := db.NewRepository(dbPath)
	defer repo.Close()

	manager, _ := engine.NewManager(repo)
	manager.Start()
	defer manager.Stop()

	server := NewServer(manager)

	t.Run("Health Check", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/reach/health", nil)
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d", rr.Code)
		}

		var resp map[string]interface{}
		json.Unmarshal(rr.Body.Bytes(), &resp)
		if resp["engine_running"] != true {
			t.Errorf("Expected engine_running true, got %v", resp["engine_running"])
		}
	})

	t.Run("Create Session via REST", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"session_id": "api-test-session",
			"url":        "about:blank",
			"deviceInfo": map[string]interface{}{
				"width": 1280,
				"height": 720,
			},
		})
		req, _ := http.NewRequest("POST", "/reach/session", bytes.NewBuffer(body))
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, req)

		if rr.Code != http.StatusCreated {
			t.Errorf("Expected 201, got %d", rr.Code)
		}
	})

	t.Run("List Sessions via REST", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/reach/sessions", nil)
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d", rr.Code)
		}
	})
}
