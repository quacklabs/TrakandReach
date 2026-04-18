import asyncio
import threading
import logging
from flask import Flask, jsonify, request
from .engine import PlaywrightService
import websockets

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

        if app is not None:
            self.init_app(app)

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

        # Start the background engine
        self.start_background_engine()

    def start_background_engine(self):
        self._thread = threading.Thread(target=self._run_event_loop, daemon=True)
        self._thread.start()

    def _run_event_loop(self):
        self.loop = asyncio.new_event_loop()
        asyncio.set_event_loop(self.loop)

        self.loop.run_until_complete(self.engine.start())

        start_server = websockets.serve(
            self.engine.handle_websocket,
            "0.0.0.0",
            self.ws_port
        )

        self.loop.run_until_complete(start_server)
        logger.info(f"Trakand Reach WebSocket server started on port {self.ws_port} ✅")

        self.loop.run_forever()

    def is_alive(self):
        return self.engine.is_running
