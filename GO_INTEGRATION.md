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

	// Initialize Manager
	manager, _ := engine.NewManager(repo)
	manager.Start()

	// Start multiple sessions
	accounts := []string{"marketing-1", "support-core", "bot-alpha"}

	for _, id := range accounts {
		session := &models.Session{
			ID: id,
			DeviceInfo: models.DeviceInfo{
				UserAgent: "Mozilla/5.0...",
				Width: 1280,
				Height: 720,
			},
			LastURL: "https://web.whatsapp.com",
		}

		inst, _ := manager.StartSession(session)

		// Each instance has its own event channel
		go func(accountID string, events chan engine.Event) {
			for ev := range events {
				log.Printf("[%s] Received event: %s", accountID, ev.Type)
			}
		}(id, inst.Events)
	}

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
	manager.SendMessage("my-account", "1234567890", "Hello from Go!")
}
```

## 3. Advanced: Binary Streaming
If you want to consume the WebP stream programmatically:
- The stream consists of binary frames.
- Each frame starts with the 8-byte header `WREACH\x00\x01\x00\x01`.
- The remaining bytes are the raw WebP image.

## 4. Environment Requirements
- The machine must have Go 1.21+ installed.
- Ensure `playwright install webkit` has been run on the host machine.
- The user running the binary must have write permissions to `~/.trakand_reach`.
