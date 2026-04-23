package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/username/trakand-reach/pkg/db"
	"github.com/username/trakand-reach/pkg/engine"
	"github.com/username/trakand-reach/pkg/models"
)

func TestListSessionsEndpoint(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "api-test-*")
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	repo, _ := db.NewRepository(dbPath)
	defer repo.Close()

	s1 := &models.Session{
		ID:        "s1",
		AccessKey: "key1",
		DeviceInfo: models.DeviceInfo{UserAgent: "ua1"},
	}
	repo.SaveSession(s1)

	manager, _ := engine.NewManagerWithDir(repo, tempDir)
	server := NewServer(manager)

	req, _ := http.NewRequest("GET", "/reach/sessions", nil)
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var sessions []models.Session
	if err := json.Unmarshal(rr.Body.Bytes(), &sessions); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if len(sessions) != 1 {
		t.Errorf("Expected 1 session, got %d", len(sessions))
	}

	if sessions[0].ID != "s1" {
		t.Errorf("Expected session ID s1, got %s", sessions[0].ID)
	}
}
