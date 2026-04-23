# Integrating Trakand Reach in Go

Trakand Reach is designed to be easily embeddable into any Go application.

## 1. Installation
Add the module to your project:
```bash
go get github.com/username/trakand-reach
```

## 2. Basic Initialization
```go
import (
	"github.com/username/trakand-reach/pkg/db"
	"github.com/username/trakand-reach/pkg/engine"
	"github.com/username/trakand-reach/pkg/models"
)

func main() {
	// Initialize Repository (SQLite)
	repo, _ := db.NewRepository("path/to/reach.db")

	// Initialize Manager (optional: specify custom base dir for sessions)
	manager, _ := engine.NewManager(repo)
    // Or use engine.NewManagerWithDir(repo, "/custom/path")

	manager.Start()

	// Start a session
    mySessionID := "your-unique-session-id"
	session := &models.Session{
		ID: mySessionID,
		DeviceInfo: models.DeviceInfo{
			UserAgent: "Mozilla/5.0...",
			Width: 1280,
			Height: 720,
            Device: models.DeviceType{
                Type: "desktop",
            },
		},
	}

	inst, _ := manager.StartSession(session)

	// Listen for events
	go func() {
		for ev := range inst.Events {
			switch ev.Type {
			case "message_new":
				// Handle intercepted message
			case "qr":
				// Handle QR code string
			}
		}
	}()

	// Send a message
	manager.SendMessage(mySessionID, "1234567890", "Hello from Go!")
}
```

## 3. Advanced: Binary Streaming
If you want to consume the WebP stream programmatically via WebSocket:
- The stream consists of binary frames.
- Each frame starts with the 8-byte header `WREACH\x00\x01\x00\x01` (Magic: WREACH, Version: 1, Type: 1/WebP).
- The remaining bytes are the raw WebP image.

## 4. REST API Reference
- `GET /reach/health`: Check engine status.
- `GET /reach/sessions`: List all managed sessions.
- `POST /reach/send`: Send a message.
  ```json
  { "session_id": "account_1", "to": "1234567890", "text": "Hello" }
  ```

## 5. Environment Requirements
- The machine must have Go 1.21+ installed.
- Ensure `playwright install webkit` has been run on the host machine.
- The user running the binary must have write permissions to the database path and the session directory (defaults to `~/.trakand_reach`).
