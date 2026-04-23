package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/username/trakand-reach/pkg/db"
	"github.com/username/trakand-reach/pkg/models"
)

func TestThrottledResume(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "reach-test-*")
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	repo, _ := db.NewRepository(dbPath)
	defer repo.Close()

	// Seed 4 sessions
	for i := 1; i <= 4; i++ {
		id := "session-" + string(rune('0'+i))
		s := &models.Session{
			ID:        id,
			LastURL:   "https://web.whatsapp.com",
			AccessKey: "test",
			DeviceInfo: models.DeviceInfo{
				UserAgent: "test",
			},
		}
		repo.SaveSession(s)
	}

	manager, _ := NewManagerWithDir(repo, tempDir)
    if manager == nil {
        t.Fatal("Manager is nil")
    }

	sessions, _ := repo.GetSessions()
	if len(sessions) != 4 {
		t.Errorf("Expected 4 sessions, got %d", len(sessions))
	}
}
