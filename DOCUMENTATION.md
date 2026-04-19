# Trakand Reach Documentation

Trakand Reach is a professional-grade Playwright orchestration engine for Python. It is designed to be a high-performance, resilient, and lightweight replacement for complex tools like the Evolution API, specifically for WhatsApp Web automation and bot development.

## Table of Contents
1. [Architecture](#architecture)
2. [Installation](#installation)
3. [Multi-Instance Management](#multi-instance-management)
4. [Event Hooks (Baileys-style)](#event-hooks)
5. [Flask Integration](#flask-integration)
6. [Resilience & Persistence](#resilience-and-persistence)
7. [CLI Reference](#cli-reference)
8. [Evolution API Comparison](#evolution-api-comparison)

---

## Architecture
Trakand Reach runs a headless WebKit browser managed by Playwright. It manages an `asyncio` event loop in a background thread when used with Flask, ensuring that browser interactions do not block your web server.

- **Engine**: The core `PlaywrightService` singleton managing browser instances.
- **WebSocket Server**: Streams live screenshots (frames) for QR code scanning and visual debugging.
- **Persistence Layer**: Automatically saves session state to `sessions.json` and uses Playwright's `launch_persistent_context` to keep you logged in.

---

## Installation

```bash
pip install git+https://github.com/username/trakand-reach.git

# One-time setup for browsers and systemd
trakand-reach setup --user youruser
```

---

## Multi-Instance Management
Unlike simple automation scripts, Trakand Reach is built for scale. Each WhatsApp account is a unique `Session` with its own persistent directory.

### Creating a new instance via HTTP
```http
POST /reach/whatsapp
Content-Type: application/json

{
  "session_id": "account_marketing_1"
}
```

### Accessing sessions in Python
```python
from trakand_reach import TrakandReach
reach = TrakandReach(app)

# List all sessions
all_sessions = reach.get_sessions()
print(f"Managing {len(all_sessions)} accounts.")
```

---

## Event Hooks
Inspired by the `Baileys` library, Trakand Reach uses an event-driven model.

| Event | Data | Description |
|---|---|---|
| `qr` | `string` | Emitted when a new WhatsApp QR code is generated. |
| `message` | `dict` | Emitted when a new message is received (`{text, sender, sender_id}`). |
| `connection` | `dict` | Emitted when the connection state changes. |

### Using Hooks in Flask
```python
@reach.on('message')
def handle_message(data):
    # data['sender_id'] contains the unique phone number
    print(f"New Message: {data['text']} from {data['sender_id']}")

    # Auto-reply logic using precision targeting
    if "help" in data['text'].lower():
        # Passing sender_id (phone number) ensures precision
        reach.send_message(session_id="...", to=data['sender_id'], text="How can I help?")
```

---

## Flask Integration
The `TrakandReach` extension is the primary way to integrate the engine into your app.

```python
from flask import Flask
from trakand_reach import TrakandReach

app = Flask(__name__)
reach = TrakandReach(app)

# All background tasks, WebSockets, and Playwright instances
# are managed automatically in a background thread.
```

---

## Resilience and Persistence
- **Auto-Resume**: When the service restarts (e.g., after a crash or system reboot), Trakand Reach automatically re-spins all sessions that were previously active and navigates back to their last URL.
- **Fast Restart**: Metadata is stored in a lightweight JSON file for sub-second state restoration.
- **Systemd**: The provided systemd service ensures the engine starts on boot and restarts automatically on failure.

---

## CLI Reference
- `trakand-reach install`: Install WebKit and dependencies.
- `trakand-reach setup`: Full environment setup + systemd installation.
- `trakand-reach run`: Start the engine in standalone mode.
- `trakand-reach whatsapp`: Quick-start a single WhatsApp Web session.
- `trakand-reach bot`: Start a sample auto-reply bot.

---

## Evolution API Comparison

| Feature | Evolution API | Trakand Reach |
|---|---|---|
| **Memory Footprint** | Heavy (Docker + Multiple services) | Lightweight (Single Python Process) |
| **Logic Control** | External Webhooks | Native Python Callbacks/Decorators |
| **Persistence** | Database Required | File-based (Zero-config) |
| **Browser** | Chromium (Heavy) | WebKit (Optimized/Lightweight) |
| **Integration** | API-only | Direct Flask Extension + API |

Trakand Reach allows you to keep your bot logic **inside** your Flask application, reducing latency and infrastructure complexity.
