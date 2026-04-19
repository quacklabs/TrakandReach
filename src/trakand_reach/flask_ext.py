import asyncio
import threading
import logging
from flask import Flask, jsonify, request
from .engine import PlaywrightService
from websockets.asyncio.server import serve as ws_serve

logger = logging.getLogger("trakand_reach.flask")

class TrakandReach:
    """
    Flask extension to integrate Trakand Reach engine.
    This manages the engine in a background thread.
    """
    def __init__(self, app: Flask = None, ws_port: int = 3000):
        self.app = app
        self.ws_port = ws_port
        self.engine = PlaywrightService()
        self.loop = None
        self._thread = None

        self.hooks = {
            'qr': [],
            'message': [],
            'connection': []
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
        app.extensions['trakand_reach'] = self

        # Register management routes on the parent Flask app
        @app.route('/reach/health', methods=['GET'])
        def health():
            return jsonify({
                "status": "ok",
                "engine_running": self.engine.is_running,
                "sessions_active": len(self.engine.sessions)
            })

        @app.route('/reach/sessions', methods=['GET'])
        def list_sessions():
            return jsonify({sid: s.to_dict() for sid, s in self.engine.sessions.items()})

        @app.route('/reach/session', methods=['POST'])
        def create_session():
            data = request.json
            if not self.loop:
                return jsonify({"error": "Engine not started"}), 500

            future = asyncio.run_coroutine_threadsafe(
                self.engine.create_session(
                    data.get('access_key'),
                    data.get('deviceInfo'),
                    data.get('browser', 'webkit')
                ),
                self.loop
            )
            session = future.result()
            return jsonify({
                "session_id": session.id,
                "ws_url": f"ws://{request.host.split(':')[0]}:{self.ws_port}"
            })

        @app.route('/reach/whatsapp', methods=['POST'])
        def start_whatsapp():
            data = request.json or {}
            # Standard WhatsApp Device Info
            device_info = {
                "userAgent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15",
                "width": 1280,
                "height": 720,
                "pixelRatio": 1.0,
                "fingerprint": data.get('session_id', 'whatsapp-session')
            }

            future = asyncio.run_coroutine_threadsafe(
                self.setup_whatsapp(device_info),
                self.loop
            )
            session_id = future.result()

            return jsonify({
                "session_id": session_id,
                "ws_url": f"ws://{request.host.split(':')[0]}:{self.ws_port}",
                "message": "WhatsApp session initiated. Connect to WebSocket to scan QR code."
            })

        @app.route('/reach/send', methods=['POST'])
        def send_message():
            data = request.json
            self.send_message(
                data.get('session_id'),
                data.get('to'),
                data.get('text')
            )
            return jsonify({"status": "sent"})

        # Start the background engine
        self.start_background_engine()

    def send_message(self, session_id, to, text):
        """Send a message through a specific session"""
        if not self.loop:
            raise RuntimeError("Engine not started")
        future = asyncio.run_coroutine_threadsafe(
            self.engine.send_whatsapp_message(session_id, to, text),
            self.loop
        )
        return future.result()

    def start_background_engine(self):
        self._thread = threading.Thread(target=self._run_event_loop, daemon=True)
        self._thread.start()

    def _run_event_loop(self):
        self.loop = asyncio.new_event_loop()
        asyncio.set_event_loop(self.loop)

        async def _run() -> None:
            await self.engine.start()
            # websockets ≥12: serve() must run inside the loop (async with), not as a bare coroutine
            # passed to run_until_complete without a running loop during Server setup.
            await asyncio.sleep(0)
            async with ws_serve(
                self.engine.handle_websocket,
                "0.0.0.0",
                self.ws_port,
            ):
                logger.info("Trakand Reach WebSocket server started on port %s ✅", self.ws_port)
                await asyncio.Future()  # keep loop alive until process exit

        self.loop.run_until_complete(_run())

    async def setup_whatsapp(self, device_info):
        session = await self.engine.create_session("whatsapp-key", device_info)

        # Register hooks into the session
        session.event_listeners['qr'].extend(self.hooks['qr'])
        session.event_listeners['message_new'].extend(self.hooks['message'])
        session.event_listeners['connection_update'].extend(self.hooks['connection'])

        # We don't block on start_up_link here to return to Flask quickly
        asyncio.create_task(self.engine.start_up_link(session.id, "https://web.whatsapp.com"))
        return session.id

    def get_sessions(self):
        """Return a list of all managed sessions"""
        return self.engine.sessions

    def get_session(self, session_id):
        """Return a specific session"""
        return self.engine.sessions.get(session_id)

    def is_alive(self):
        return self.engine.is_running
