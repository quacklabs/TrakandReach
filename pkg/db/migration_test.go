package db

import (
	"os"
	"testing"
)

func TestMigration(t *testing.T) {
	dbPath := "./test_migrate.db"
	jsonPath := "./sessions.json"
	defer os.Remove(dbPath)
	defer os.Remove(jsonPath)
	defer os.Remove(jsonPath + ".bak")

	legacyJSON := `{
		"legacy-id": {
			"id": "legacy-id",
			"browser_type": "webkit",
			"access_key": "old-key",
			"last_url": "https://web.whatsapp.com",
			"connection_state": "open",
			"device_info": {
				"os": "macOS",
				"userAgent": "Mozilla/5.0",
				"browser": "webkit",
				"width": 1280,
				"height": 720,
				"pixelRatio": 1,
				"dark_mode": true,
				"language": "en-US",
				"device": {
					"type": "desktop",
					"model": "MacBook",
					"brand": "Apple",
					"os": "macOS"
				}
			}
		}
	}`
	os.WriteFile(jsonPath, []byte(legacyJSON), 0644)

	repo, _ := NewRepository(dbPath)
	defer repo.Close()

	err := repo.ImportFromJSON(jsonPath)
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	sessions, _ := repo.GetSessions()
	if len(sessions) != 1 {
		t.Errorf("Expected 1 session, got %d", len(sessions))
	}

	if sessions[0].ID != "legacy-id" {
		t.Errorf("Expected legacy-id, got %s", sessions[0].ID)
	}

	if _, err := os.Stat(jsonPath); !os.IsNotExist(err) {
		t.Errorf("Original JSON file should be moved/renamed")
	}
}
