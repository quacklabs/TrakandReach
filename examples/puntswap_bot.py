from flask import Flask, jsonify, request
from trakand_reach import TrakandReach
import logging

# Trakand Reach: PuntSwap Platform Integration Example
# This example demonstrates how to route WhatsApp messages to a PuntSwap logic handler

logging.basicConfig(level=logging.INFO)

app = Flask(__name__)
reach = TrakandReach(app)

class PuntSwapHandler:
    @staticmethod
    def process(text, sender_id):
        text = text.lower()
        if "balance" in text:
            return f"Your PuntSwap balance is: 0.00 BTC"
        elif "trade" in text:
            return "PuntSwap Trading is active. Use 'trade <asset> <amount>'"
        elif "help" in text:
            return "Welcome to PuntSwap on WhatsApp! Available commands: balance, trade, help"
        return "PuntSwap: Command not recognized. Type 'help' for options."

@reach.on('message')
def on_whatsapp_message(data):
    text = data.get('text', '')
    sender_id = data.get('sender_id', 'unknown')

    print(f"PuntSwap routing: Message from {sender_id}: {text}")

    # Process through PuntSwap Platform logic
    response = PuntSwapHandler.process(text, sender_id)

    # Send reply back via Trakand Reach
    reach.send_message(
        session_id="puntswap-session",
        to=sender_id,
        text=response
    )

@app.route("/puntswap/init")
def init_puntswap():
    reach.setup_whatsapp({"fingerprint": "puntswap-session"})
    return jsonify({"status": "initializing", "platform": "PuntSwap"})

if __name__ == "__main__":
    app.run(port=5000)
