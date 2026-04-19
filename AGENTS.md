# Integration Guide for Agents: Trakand Reach

This document provides a precise, step-by-step guide for an AI agent to integrate the **Trakand Reach** library into an existing Flask application. Follow these steps to enable multi-instance WhatsApp bot capabilities.

## 1. Environment Setup
The agent must ensure the following commands are executed in the project environment:

```bash
# Install the library from the local source or git repo
pip install .

# Install necessary Playwright browsers and system dependencies
trakand-reach setup --user your_linux_user
```

## 2. Initialization in Flask
To integrate Trakand Reach, modify the main Flask application file (e.g., `app.py`).

```python
from flask import Flask
from trakand_reach import TrakandReach

app = Flask(__name__)

# Initialize the extension.
# This automatically starts the background asyncio loop and the WebSocket server (default port 3000).
reach = TrakandReach(app, ws_port=3000)
```

## 3. Implementing Bot Logic (Hooks)
Use the `@reach.on` decorator to listen for events. This is where the core bot logic lives.

```python
@reach.on('message')
def handle_whatsapp_message(data):
    """
    Triggered whenever any managed instance receives a message.
    'data' contains: {text, sender, sender_id}
    'sender_id' is the unique phone number/identifier.
    """
    text = data.get('text', '')
    sender_id = data.get('sender_id')

    # Example Auto-Reply
    if "hello" in text.lower():
        # reach.send_message is thread-safe and can be called from here
        reach.send_message(
            session_id="whatsapp-session", # The ID you used to spawn the instance
            to=sender_id,
            text="Hello! How can I assist you today?"
        )
```

## 4. Spawning Instances
You can spawn new WhatsApp instances programmatically via a Flask route or a background task.

```python
@app.route("/init-session/<name>")
def init_session(name):
    # This initiates a new persistent browser instance and navigates to WhatsApp Web
    reach.setup_whatsapp({
        "fingerprint": name # Unique ID for this account (e.g., 'marketing-1')
    })
    return {"status": "scanning", "instance": name}
```

## 5. Key API Methods for Agents

| Method | Description |
|---|---|
| `reach.setup_whatsapp(device_info)` | Starts a new WhatsApp session. Requires `fingerprint` in dict. |
| `reach.send_message(session_id, to, text)` | Sends a message. `to` can be a name or a phone number. |
| `reach.get_sessions()` | Returns a dictionary of all active `Session` objects. |
| `reach.get_session(session_id)` | Returns a specific session object. |

## 6. Verification Checklist
- [ ] Browser binaries are installed (`trakand-reach install`).
- [ ] TrakandReach is initialized with the Flask `app` instance.
- [ ] At least one `@reach.on('message')` hook is registered for bot logic.
- [ ] The engine is started (happens automatically on `init_app`).
- [ ] QR codes are scanned via the WebSocket stream (default `ws://localhost:3000`).

## 7. Troubleshooting for Agents
- **Async Errors**: Never call `asyncio` methods of the engine directly. Always use the `reach` methods or `run_coroutine_threadsafe` if you are extending the core.
- **Session Persistence**: Browser data is stored in `~/.trakand_reach/browserSessions`. If a session is corrupted, delete its folder there.
- **Port Conflicts**: If port 3000 is taken, change the `ws_port` in the `TrakandReach` constructor.
