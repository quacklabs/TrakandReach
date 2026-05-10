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

	// Start a session
	session := &models.Session{
		ID: "my-account",
		DeviceInfo: models.DeviceInfo{
			UserAgent: "Mozilla/5.0...",
			Width: 1280,
			Height: 720,
		},
		LastURL: "https://web.whatsapp.com",
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
	manager.SendMessage("my-account", "1234567890", "Hello from Go!")
}
```

## 3. Sidecar Integration (HTTP)

When running Trakand Reach as a sidecar, you can interact with it via its REST API (default port 3000).

### Query Engine Status
`GET /reach/health`
Returns the current health and boot state of the engine.

### Start a New Session
`POST /reach/sessions`
```json
{
  "id": "unique-session-id",
  "webhook_url": "https://your-backend.com/webhook",
  "device_info": {
    "userAgent": "...",
    "width": 1280,
    "height": 720
  }
}
```

### Get QR Code
`GET /reach/sessions/{id}/qr`
Returns the latest generated QR code for the session (if not already authenticated).

### Send a Message
`POST /reach/send`
```json
{
  "session_id": "unique-session-id",
  "to": "1234567890",
  "text": "Hello from Sidecar!"
}
```

## 4. Spam Filtering

Trakand Reach includes a high-performance spam filter. It reads from a `spam.txt` file in the application's working directory.

### `spam.txt` Format
```text
# Block messages from specific numbers
phone: 1234567890
phone: 0987654321

# Block messages containing specific keywords (case-insensitive)
word: buy now
word: click here
word: spammy-link.com
```

Filtered messages are discarded and never forwarded to webhooks or emitted as events.

## 5. Advanced: Binary Streaming
If you want to consume the WebP stream programmatically:
- The stream consists of binary frames.
- Each frame starts with the 8-byte header `WREACH\x00\x01\x00\x01`.
- The remaining bytes are the raw WebP image.

## 4. Environment Requirements
- The machine must have Go 1.21+ installed.
- Ensure `playwright install webkit` has been run on the host machine.
- The user running the binary must have write permissions to `~/.trakand_reach`.
