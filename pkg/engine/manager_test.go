package engine

import (
	"os"
	"testing"

	"github.com/playwright-community/playwright-go"
	"github.com/username/trakand-reach/pkg/db"
	"github.com/username/trakand-reach/pkg/models"
)

func TestEngineManager(t *testing.T) {
	// Setup DB
	dbPath := "./test_engine.db"
	defer os.Remove(dbPath)
	repo, _ := db.NewRepository(dbPath)
	defer repo.Close()

	manager, err := NewManager(repo)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// We need Playwright for this test, but we can't easily mock the whole browser in this environment
	// without installing it. Since 'trakand-reach install' hasn't run yet, we check if pw can start.
	err = playwright.Install()
	if err != nil {
		t.Logf("Skipping full engine test as playwright browsers are not installed: %v", err)
		return
	}

	err = manager.Start()
	if err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}
	defer manager.Stop()

	s := &models.Session{
		ID: "test-engine-session",
		DeviceInfo: models.DeviceInfo{
			UserAgent: "Mozilla/5.0",
			Width:     1280,
			Height:    720,
		},
	}

	inst, err := manager.StartSession(s)
	if err != nil {
		t.Fatalf("Failed to start session: %v", err)
	}

	if inst.Page == nil {
		t.Fatal("Page is nil")
	}

	if !inst.IsAlive {
		t.Fatal("Session is not alive")
	}
}
