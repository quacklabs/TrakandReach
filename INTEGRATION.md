# Integrating TrakandReach into another application

This document is for **consumer projects** (e.g. a Flask API) that depend on TrakandReach. All packaging, pins, and deploy scripts belong in **your** repository—not in the TrakandReach tree.

## 1. Declare the dependency (your `requirements.txt`)

Pick one strategy.

### Track `main` (no commit pin)

Each fresh `pip install -r requirements.txt` resolves the branch tip at install time. To **refresh** an existing venv after upstream merges, upgrade that line explicitly:

```bash
pip install --upgrade "trakand-reach @ git+https://github.com/quacklabs/TrakandReach.git@main"
```

Example line in your requirements file:

```text
trakand-reach @ git+https://github.com/quacklabs/TrakandReach.git@main
```

Optional deploy pattern (single source of truth: keep the line above in `requirements.txt`, then):

```bash
pip install -r requirements.txt -q
tr_spec="$(awk '/^[[:space:]]*trakand-reach[[:space:]]+@/ {gsub(/^[[:space:]]+/,""); print; exit}' requirements.txt | tr -d '\r')"
if [ -n "${tr_spec}" ]; then
  pip install --upgrade -q "${tr_spec}"
fi
```

### Pin a commit (reproducible builds)

```text
trakand-reach @ git+https://github.com/quacklabs/TrakandReach.git@<40-char-sha>
```

## 2. System dependencies

- **Python**: `>=3.8` (see TrakandReach `pyproject.toml`).
- **Playwright**: after installing the package, install browsers (WebKit is the default engine used by the examples):

  ```bash
  python -m playwright install webkit
  ```

- **websockets**: TrakandReach supports **websockets 10.4+** (compat layer included). Your resolver should not pin below 10.4.

## 3. Flask extension

```python
from trakand_reach import TrakandReach

reach = TrakandReach(app, ws_port=3000)  # or reach.init_app(app)
```

Register `app.extensions["trakand_reach"]` only in **your** app code if you need a stable lookup key.

## 4. When is it safe to talk to the engine?

The extension starts an **asyncio loop in a daemon thread**. Important details:

- **`reach.loop`** is assigned only **after** that loop has started running. Do not call `asyncio.run_coroutine_threadsafe(..., reach.loop)` until the loop exists **and** the engine is ready.
- Prefer **`reach.is_engine_ready()`** (returns true when the loop is running, `PlaywrightService.start()` has completed, and the WebSocket server is bound). If you integrate against an older TrakandReach revision without this helper, fall back to checking `reach.loop` and `reach.engine.is_running`.

Example guard before scheduling coroutines from a worker or HTTP thread:

```python
reach = app.extensions.get("trakand_reach")
if reach is None or not reach.is_engine_ready():
    raise RuntimeError("TrakandReach engine not ready")
fut = asyncio.run_coroutine_threadsafe(my_coro(), reach.loop)
fut.result(timeout=120.0)
```

## 5. Hooks and multi-session

- Use `@reach.on("message")` (and `qr` / `connection`) for Flask-level hooks.
- Session-level events live on each `Session` in the engine; your app may patch `Session.emit` if you need extra fields (e.g. a stable session id on every `message_new` payload).

## 6. Standalone (no Flask)

```bash
python -m trakand_reach.cli run --port 3000
```

Or drive `PlaywrightService` from your own `asyncio.run(...)` entrypoint; see `src/trakand_reach/cli.py` and `DOCUMENTATION.md`.

## 7. Health and admin routes

When using `TrakandReach(app)`, the extension registers routes such as `/reach/health` and `/reach/whatsapp` on **your** Flask app. Coordinate paths with your API prefix or reverse proxy.

---

For behaviour and architecture of the engine itself, see [DOCUMENTATION.md](DOCUMENTATION.md).
