from flask import Flask, jsonify, request
from trakand_reach import TrakandReach
import logging

# This example demonstrates how to use Trakand Reach to build a high-precision
# bot that can replace Evolution API. It identifies customers by their unique
# ID (phone number) and replies directly using the direct URL scheme.

logging.basicConfig(level=logging.INFO)

app = Flask(__name__)
reach = TrakandReach(app, ws_port=3000)

@reach.on('message')
def on_message(data):
    text = data.get('text', '')
    sender = data.get('sender', 'unknown')
    sender_id = data.get('sender_id', 'unknown') # Extracted phone number

    print(f"Received from {sender} ({sender_id}): {text}")

    # Precise Auto-Reply Logic
    if "order status" in text.lower():
        print(f"Processing order status for {sender_id}...")

        # We use sender_id directly. reach.send_message will recognize
        # it as a phone number and use the direct precision URL.
        reach.send_message(
            session_id="whatsapp-session", # Assuming this is your active session
            to=sender_id,
            text=f"Hi {sender}, your order #12345 is currently being processed!"
        )

@app.route("/broadcast", methods=['POST'])
def broadcast():
    """Example of a mass messaging endpoint"""
    data = request.json
    numbers = data.get('numbers', []) # List of phone numbers
    message = data.get('message', '')

    results = []
    for number in numbers:
        try:
            reach.send_message("whatsapp-session", to=number, text=message)
            results.append({"number": number, "status": "sent"})
        except Exception as e:
            results.append({"number": number, "status": "failed", "error": str(e)})

    return jsonify({"results": results})

if __name__ == "__main__":
    # Start the app
    # After starting, navigate to http://localhost:5000/reach/whatsapp (POST)
    # to initiate your session and scan the QR code.
    app.run(port=5000)
