package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/username/trakand-reach/pkg/models"
	_ "github.com/mattn/go-sqlite3"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(dbPath string) (*Repository, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
	if err != nil {
		return nil, err
	}

	repo := &Repository{db: db}
	if err := repo.migrate(); err != nil {
		return nil, err
	}

	return repo, nil
}

func (r *Repository) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			browser_type TEXT DEFAULT 'webkit',
			access_key TEXT NOT NULL,
			last_url TEXT,
			webhook_url TEXT,
			connection_state TEXT DEFAULT 'created',
			owner_jid TEXT,
			profile_name TEXT,
			profile_picture_url TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS device_info (
			session_id TEXT PRIMARY KEY,
			os TEXT,
			user_agent TEXT,
			browser TEXT,
			product TEXT,
			manufacturer TEXT,
			engine TEXT,
			width INTEGER,
			height INTEGER,
			pixel_ratio REAL,
			dark_mode BOOLEAN,
			language TEXT,
			device_type TEXT,
			device_model TEXT,
			device_brand TEXT,
			device_os TEXT,
			FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS message_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT,
			msg_id TEXT UNIQUE,
			remote_jid TEXT,
			body TEXT,
			type TEXT,
			from_me BOOLEAN,
			timestamp INTEGER,
			FOREIGN KEY(session_id) REFERENCES sessions(id)
		);`,
	}

	for _, q := range queries {
		if _, err := r.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) SaveSession(s *models.Session) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		INSERT INTO sessions (id, created_at, browser_type, access_key, last_url, webhook_url, connection_state, owner_jid, profile_name, profile_picture_url)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			last_url=excluded.last_url,
			webhook_url=excluded.webhook_url,
			connection_state=excluded.connection_state,
			owner_jid=excluded.owner_jid,
			profile_name=excluded.profile_name,
			profile_picture_url=excluded.profile_picture_url
	`, s.ID, s.CreatedAt, s.BrowserType, s.AccessKey, s.LastURL, s.WebhookURL, s.ConnectionState, s.OwnerJID, s.ProfileName, s.ProfilePictureURL)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`
		INSERT INTO device_info (session_id, os, user_agent, browser, product, manufacturer, engine, width, height, pixel_ratio, dark_mode, language, device_type, device_model, device_brand, device_os)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET
			user_agent=excluded.user_agent,
			width=excluded.width,
			height=excluded.height
	`, s.ID, s.DeviceInfo.OS, s.DeviceInfo.UserAgent, s.DeviceInfo.Browser, s.DeviceInfo.Product, s.DeviceInfo.Manufacturer, s.DeviceInfo.Engine, s.DeviceInfo.Width, s.DeviceInfo.Height, s.DeviceInfo.PixelRatio, s.DeviceInfo.DarkMode, s.DeviceInfo.Language, s.DeviceInfo.Device.Type, s.DeviceInfo.Device.Model, s.DeviceInfo.Device.Brand, s.DeviceInfo.Device.OS)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (r *Repository) GetSessions() ([]*models.Session, error) {
	rows, err := r.db.Query(`
		SELECT s.id, s.created_at, s.browser_type, s.access_key, s.last_url, s.webhook_url, s.connection_state, s.owner_jid, s.profile_name, s.profile_picture_url,
		       d.os, d.user_agent, d.browser, d.product, d.manufacturer, d.engine, d.width, d.height, d.pixel_ratio, d.dark_mode, d.language, d.device_type, d.device_model, d.device_brand, d.device_os
		FROM sessions s
		JOIN device_info d ON s.id = d.session_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*models.Session
	for rows.Next() {
		s := &models.Session{}
		d := models.DeviceInfo{}
		var createdAt string
		err := rows.Scan(
			&s.ID, &createdAt, &s.BrowserType, &s.AccessKey, &s.LastURL, &s.WebhookURL, &s.ConnectionState, &s.OwnerJID, &s.ProfileName, &s.ProfilePictureURL,
			&d.OS, &d.UserAgent, &d.Browser, &d.Product, &d.Manufacturer, &d.Engine, &d.Width, &d.Height, &d.PixelRatio, &d.DarkMode, &d.Language, &d.Device.Type, &d.Device.Model, &d.Device.Brand, &d.Device.OS,
		)
		if err != nil {
			return nil, err
		}
		// SQLite might return different time formats; simple parse for now
		s.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		if s.CreatedAt.IsZero() {
			s.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		}
		d.Fingerprint = s.ID
		s.DeviceInfo = d
		sessions = append(sessions, s)
	}
	return sessions, nil
}

func (r *Repository) Close() error {
	return r.db.Close()
}

// Migration from legacy JSON
func (r *Repository) ImportFromJSON(jsonPath string) error {
	if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
		return nil
	}

	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return err
	}

	var legacySessions map[string]interface{}
	if err := json.Unmarshal(data, &legacySessions); err != nil {
		return err
	}

	for id, val := range legacySessions {
		sdata := val.(map[string]interface{})
		dData := sdata["device_info"].(map[string]interface{})
		dtData := dData["device"].(map[string]interface{})

		s := &models.Session{
			ID:                id,
			BrowserType:       sdata["browser_type"].(string),
			AccessKey:         sdata["access_key"].(string),
			LastURL:           fmt.Sprintf("%v", sdata["last_url"]),
			ConnectionState:   fmt.Sprintf("%v", sdata["connection_state"]),
			CreatedAt:         time.Now(), // approximate
		}

		s.DeviceInfo = models.DeviceInfo{
			OS:           fmt.Sprintf("%v", dData["os"]),
			UserAgent:    fmt.Sprintf("%v", dData["userAgent"]),
			Browser:      fmt.Sprintf("%v", dData["browser"]),
			Width:        int(dData["width"].(float64)),
			Height:       int(dData["height"].(float64)),
			PixelRatio:   dData["pixelRatio"].(float64),
			DarkMode:     dData["dark_mode"].(bool),
			Language:     fmt.Sprintf("%v", dData["language"]),
			Device: models.DeviceType{
				Type:  fmt.Sprintf("%v", dtData["type"]),
				Model: fmt.Sprintf("%v", dtData["model"]),
				Brand: fmt.Sprintf("%v", dtData["brand"]),
				OS:    fmt.Sprintf("%v", dtData["os"]),
			},
		}

		if err := r.SaveSession(s); err != nil {
			return err
		}
	}

	return os.Rename(jsonPath, jsonPath+".bak")
}
