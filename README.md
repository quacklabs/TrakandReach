# Trakand Reach (Go Port)

A high-performance, Evolution-grade Playwright orchestration engine written in Go. Designed for heavy-duty WhatsApp automation and microservice integration.

## Key Features
- **Evolution-API Grade**: Deep interception of WhatsApp messages via internal Store hooks.
- **Microservice Ready**: Built-in webhook dispatcher with retry logic.
- **Optimized Streaming**: Real-time WebP screenshot streaming via Binary WebSockets.
- **Persistence**: SQLite 3 with WAL mode for high concurrency.
- **Hands-free Deployment**: Automatic systemd service installation and management.
- **Playwright Powered**: Uses WebKit for a lightweight yet powerful browser automation experience.

## Installation

### 1. Build and Install
```bash
go build -o trakand-reach ./cmd/trakand-reach/main.go
./trakand-reach install
```

### 2. Setup as System Service
```bash
sudo ./trakand-reach setup --port 3000
```

## Usage as a Microservice

### REST API
- `POST /reach/send`: Send a message.
  ```json
  {
    "session_id": "account_1",
    "to": "1234567890",
    "text": "Hello from Trakand Reach Go!"
  }
  ```

### WebSocket (Binary)
Connect to `ws://localhost:3000/ws` to receive real-time binary WebP frames and events.

## Usage as a Library

```go
import "github.com/username/trakand-reach/pkg/reach"

// Implementation details in GO_INTEGRATION.md
```

## License
MIT
