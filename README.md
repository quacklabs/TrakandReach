# Trakand Reach

A Playwright orchestration engine for Python, designed for high-performance browser automation and streaming.

## Features

- **Full Documentation**: See [DOCUMENTATION.md](DOCUMENTATION.md) for a deep dive.
- **Asynchronous Orchestration**: Built on Python's `asyncio` and Playwright for maximum efficiency.
- **WebSocket Streaming**: Real-time screenshot streaming via resilient WebSockets.
- **Flask Integration**: Easy-to-use Flask extension for managing browser sessions.
- **Standalone Mode**: Run as a standalone server with an integrated HTTP and WebSocket API.
- **Systemd Support**: Built-in CLI for setting up as a Linux system service.
- **Automatic Browser Management**: Installs WebKit automatically on the first run.

## Installation

Install directly from the private GitHub repository:

```bash
pip install git+https://github.com/username/trakand-reach.git
```

## Usage

### Flask Extension

```python
from flask import Flask
from trakand_reach import TrakandReach

app = Flask(__name__)
reach = TrakandReach(app)

@app.route("/health")
def health():
    return {"status": "ok", "engine": reach.is_alive()}

if __name__ == "__main__":
    app.run()
```

### Standalone Mode

```bash
trakand-reach run --port 3000
```

### Systemd Setup

```bash
trakand-reach setup-service --user $(whoami)
```

## License

MIT
