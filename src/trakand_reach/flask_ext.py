from __future__ import annotations

import asyncio
import threading
import logging
from typing import Optional

from flask import Flask, jsonify, request

from .engine import PlaywrightService
from ._ws_compat import websocket_serve

logger = logging.getLogger("trakand_reach.flask")


def _extend_unique(target: list, items: list) -> None:
    for item in items:
        if item not in target:
            target.append(item)


class TrakandReach:
    """
    Flask extension to integrate Trakand Reach engine.
    This manages the engine in a background thread.
    """

    def __init__(self, app: Flask = None, ws_port: int = 3000):
        self.app = app
        self.ws_port = ws_port
        self.engine = PlaywrightService()
        self.loop: Optional[asyncio.AbstractEventLoop] = None
        self._thread: threading.Thread | None = None
        self._loop_ready_event = threading.Event()
        self._init_app_done = False

        self.hooks = {
            "qr": [],
            "message": [],
            "connection": [],
        }
        if app is not None:
            self.init_app(app)

    def on(self, event_name: str):
        """Decorator to register hooks"""

        def decorator(f):
            if event_name in self.hooks:
                self.hooks[event_name].append(f)
            return f

        return decorator

    def init_app(self, app: Flask):
        self.app = app
        prev = app.extensions.get("trakand_reach")
        if prev is not None and prev is not self:
            logger.warning(
                "Flask app already has a different trakand_reach extension; "
                "skipping init_app on this TrakandReach instance."
            )
            return

        app.extensions["trakand_reach"] = self

        if self._init_app_done:
            return
        self._init_app_done = True

        @app.route("/reach/health", methods=["GET"])
        def health():
            return jsonify(
                {
                    "status": "ok",
                    "engine_running": self.engine.is_running,
                    "sessions_active": len(self.engine.sessions),
                }
            )

        @app.route("/reach/sessions", methods=["GET"])
        def list_sessions():
            return jsonify({sid: s.to_dict() for sid, s in self.engine.sessions.items()})

        @app.route("/reach/session", methods=["POST"])
        def create_session():
            data = request.json
            if not self._loop_ready_event.is_set() or not self.loop:
                return jsonify({"error": "Engine not started"}), 500

            future = asyncio.run_coroutine_threadsafe(
                self.engine.create_session(
                    data.get("access_key"),
                    data.get("deviceInfo"),
                    data.get("browser", "webkit"),
                ),
                self.loop,
            )
            session = future.result()
            return jsonify(
                {
                    "session_id": session.id,
                    "ws_url": f"ws://{request.host.split(':')[0]}:{self.ws_port}",
                }
            )

        @app.route("/reach/whatsapp", methods=["POST"])
        def start_whatsapp():
            data = request.json or {}
            device_info = {
                "userAgent": (
                    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 "
                    "(KHTML, like Gecko) Version/17.0 Safari/605.1.15"
                ),
                "width": 1280,
                "height": 720,
                "pixelRatio": 1.0,
                "fingerprint": data.get("session_id", "whatsapp-session"),
            }

            if not self._loop_ready_event.is_set() or not self.loop:
                return jsonify({"error": "Engine not started"}), 500

            future = asyncio.run_coroutine_threadsafe(
                self.setup_whatsapp(device_info),
                self.loop,
            )
            session_id = future.result()

            return jsonify(
                {
                    "session_id": session_id,
                    "ws_url": f"ws://{request.host.split(':')[0]}:{self.ws_port}",
                    "message": "WhatsApp session initiated. Connect to WebSocket to scan QR code.",
                }
            )

        @app.route("/reach/send", methods=["POST"])
        def send_message_route():
            data = request.json
            try:
                self.send_message(
                    data.get("session_id"),
                    data.get("to"),
                    data.get("text"),
                )
            except Exception as e:
                return jsonify({"error": str(e)}), 500
            return jsonify({"status": "sent"})

        self.start_background_engine()

    def send_message(self, session_id, to, text, timeout: float = 120.0):
        """Send a message through a specific session."""
        if not self.loop or not self._loop_ready_event.is_set():
            raise RuntimeError("Engine not started")
        future = asyncio.run_coroutine_threadsafe(
            self.engine.send_whatsapp_message(session_id, to, text),
            self.loop,
        )
        return future.result(timeout=timeout)

    def start_background_engine(self):
        if self._thread is not None and self._thread.is_alive():
            return
        self._loop_ready_event.clear()
        self._thread = threading.Thread(
            target=self._run_event_loop,
            daemon=True,
            name="trakand-reach-asyncio",
        )
        self._thread.start()

    def _run_event_loop(self):
        loop = asyncio.new_event_loop()
        asyncio.set_event_loop(loop)

        async def _run() -> None:
            # Expose only after the loop is running (safe for run_coroutine_threadsafe).
            self.loop = loop
            try:
                await self.engine.start()
            except Exception:
                logger.exception("TrakandReach engine failed to start")
                raise
            self._loop_ready_event.set()
            async with websocket_serve(
                self.engine.handle_websocket,
                "0.0.0.0",
                self.ws_port,
            ):
                logger.info(
                    "Trakand Reach WebSocket server started on port %s ✅",
                    self.ws_port,
                )
                await asyncio.Future()

        try:
            loop.run_until_complete(_run())
        except Exception:
            logger.exception("TrakandReach background event loop exited with an error")
        finally:
            self._loop_ready_event.clear()
            self.loop = None

    async def setup_whatsapp(self, device_info):
        session = await self.engine.create_session("whatsapp-key", device_info)

        _extend_unique(session.event_listeners["qr"], self.hooks["qr"])
        _extend_unique(session.event_listeners["message_new"], self.hooks["message"])
        _extend_unique(
            session.event_listeners["connection_update"], self.hooks["connection"]
        )

        asyncio.create_task(
            self.engine.start_up_link(session.id, "https://web.whatsapp.com")
        )
        return session.id

    def get_sessions(self):
        """Return a list of all managed sessions"""
        return self.engine.sessions

    def get_session(self, session_id):
        """Return a specific session"""
        return self.engine.sessions.get(session_id)

    def is_alive(self):
        return self.engine.is_running

    def is_engine_ready(self) -> bool:
        """True when the background loop is running, Playwright has started, and the WS server is up."""
        return (
            self.loop is not None
            and self._loop_ready_event.is_set()
            and self.engine.is_running
        )
