# Trakand Reach Documentation (Go)

## Architecture
Trakand Reach Go is built on a concurrent, event-driven architecture. Each WhatsApp session runs in its own goroutine, managing a persistent Playwright context.

### Deep Interception
Unlike standard scrapers, Trakand Reach hooks into the internal `WWebStore` of WhatsApp Web. This allows for:
- **Instant Message Detection**: Intercepting messages at the data layer.
- **Background Sending**: Sending messages without UI interaction.
- **State Monitoring**: Real-time tracking of QR and connection status.

## CLI Reference
- `install`: Downloads required browser binaries.
- `setup`: Installs the engine as a systemd service and starts it.
- `uninstall`: Gracefully removes the systemd service.
- `run`: Starts the engine in the foreground.
- `whatsapp`: Quick-start a standalone WhatsApp session.
- `bot`: Runs a sample auto-reply bot.

## Persistence
All session metadata and message logs are stored in `~/.trakand_reach/reach.db`. Browser sessions (cookies, localStorage) are stored in `~/.trakand_reach/browserSessions/`.

## Webhooks
When initializing a session, you can provide a `webhook_url`. The engine will POST every intercepted message to this URL with an exponential backoff retry policy.
