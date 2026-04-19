from flask import Flask, jsonify, request
from trakand_reach import TrakandReach
import logging

# Setup basic logging
logging.basicConfig(level=logging.INFO)

app = Flask(__name__)
# Initialize Trakand Reach with the Flask app
reach = TrakandReach(app, ws_port=3000)

@reach.on('qr')
def handle_qr(data):
    # This will be called whenever a QR code is generated for ANY session
    # 'data' is the raw QR code string (data-ref)
    print(f"\n[GLOBAL QR HOOK] New QR code generated: {data[:20]}...")

@reach.on('message')
def handle_message(data):
    # This is called for every incoming message across all instances
    text = data.get('text', '')
    sender = data.get('sender', 'unknown')
    print(f"\n[GLOBAL MESSAGE HOOK] {sender} says: {text}")

    # Example: Simple auto-reply bot
    if "ping" in text.lower():
        # You would typically find the session_id associated with the bot
        # For this example, we'll just log it
        print(f"Responding to ping from {sender}...")

@app.route("/")
def index():
    sessions = reach.get_sessions()
    return jsonify({
        "managed_instances": len(sessions),
        "instances": [sid for sid in sessions]
    })

@app.route("/spawn/<instance_id>")
def spawn(instance_id):
    """Spawn a new WhatsApp instance with a custom ID"""
    reach.setup_whatsapp({
        "userAgent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15",
        "width": 1280,
        "height": 720,
        "pixelRatio": 1.0,
        "fingerprint": instance_id
    })
    return jsonify({"status": "spawning", "instance": instance_id})

if __name__ == "__main__":
    # Run the Flask app
    # Note: reach.start_background_engine() is called automatically during init_app
    app.run(port=5000)
