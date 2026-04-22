package db

import (
	"os"
	"testing"
	"time"

	"github.com/username/trakand-reach/pkg/models"
)

func TestRepository(t *testing.T) {
	dbPath := "./test_reach.db"
	defer os.Remove(dbPath)

	repo, err := NewRepository(dbPath)
	if err != nil {
		t.Fatalf("Failed to create repo: %v", err)
	}
	defer repo.Close()

	s := &models.Session{
		ID:          "test-session",
		CreatedAt:   time.Now().Round(time.Second),
		BrowserType: "webkit",
		AccessKey:   "key123",
		DeviceInfo: models.DeviceInfo{
			UserAgent: "Mozilla/5.0",
			Width:     1280,
			Height:    720,
			Device: models.DeviceType{
				Type: "desktop",
			},
		},
	}

	if err := repo.SaveSession(s); err != nil {
		t.Errorf("Failed to save session: %v", err)
	}

	sessions, err := repo.GetSessions()
	if err != nil {
		t.Errorf("Failed to get sessions: %v", err)
	}

	if len(sessions) != 1 {
		t.Errorf("Expected 1 session, got %d", len(sessions))
	}

	if sessions[0].ID != s.ID {
		t.Errorf("ID mismatch: %s != %s", sessions[0].ID, s.ID)
	}
}
