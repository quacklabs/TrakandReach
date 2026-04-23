# Trakand Reach Documentation (Go)

## Architecture
Trakand Reach Go is built on a concurrent, event-driven architecture. Each WhatsApp session runs in its own goroutine, managing a persistent Playwright context.

### Deep Interception
Trakand Reach hooks into the internal `WWebStore` of WhatsApp Web using multiple detection strategies for maximum resilience. This allows for:
- **Instant Message Detection**: Intercepting messages at the data layer.
- **High-Reliability Sending**: Sending messages using internal WhatsApp methods, bypassing UI interaction when possible.
- **State Monitoring**: Real-time tracking of QR and connection status.

## CLI Reference
- `install`: Downloads required browser binaries.
- `setup`: Installs the engine as a systemd service and starts it.
- `uninstall`: Gracefully removes the systemd service.
- `run`: Starts the engine in the foreground.
- `whatsapp`: Quick-start a standalone WhatsApp session.
- `bot`: Runs a sample auto-reply bot.

## Persistence
All session metadata and message logs are stored in the provided SQLite database. Browser sessions (cookies, localStorage) are stored in the `browserSessions/` subdirectory relative to the engine's base directory (defaults to `~/.trakand_reach`).

## Resource Management
- **Throttled Resume**: When starting up, the engine resumes saved sessions in small batches to prevent CPU and memory spikes.
- **One Client Policy**: The WebSocket server ensures only one client is connected to a specific session at a time to prevent event conflicts.

## Webhooks
When initializing a session, you can provide a `webhook_url`. The engine will POST every intercepted message to this URL with an exponential backoff retry policy.
