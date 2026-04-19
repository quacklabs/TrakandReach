import click
import asyncio
import signal
import sys
import os
import logging

from .engine import PlaywrightService
from ._ws_compat import websocket_serve

logger = logging.getLogger("trakand_reach.cli")


@click.group()
def main():
    """Trakand Reach CLI"""
    pass


@main.command()
@click.option("--port", default=3000, help="WebSocket port")
@click.option("--host", default="0.0.0.0", help="Host to bind to")
def run(port, host):
    """Run Trakand Reach in lightweight standalone mode"""

    async def _serve() -> None:
        engine = PlaywrightService()
        stop = asyncio.Event()

        def _request_stop() -> None:
            stop.set()

        loop = asyncio.get_running_loop()
        for sig in (signal.SIGTERM, signal.SIGINT):
            try:
                loop.add_signal_handler(sig, _request_stop)
            except (NotImplementedError, AttributeError, ValueError):
                pass
        if hasattr(signal, "SIGHUP"):
            try:
                loop.add_signal_handler(signal.SIGHUP, _request_stop)
            except (NotImplementedError, AttributeError, ValueError):
                pass

        await engine.start()
        try:
            async with websocket_serve(engine.handle_websocket, host, port):
                click.echo("Trakand Reach Standalone Service started ✅")
                click.echo(f"WebSocket: ws://{host}:{port}")
                await stop.wait()
        finally:
            await engine.stop()

    try:
        asyncio.run(_serve())
    except KeyboardInterrupt:
        pass
    click.echo("Service stopped.")


@main.command()
def install():
    """Install Playwright browsers and dependencies"""
    click.echo("Installing Playwright browsers (WebKit)...")
    os.system(f"{sys.executable} -m playwright install webkit")
    os.system(f"{sys.executable} -m playwright install-deps webkit")
    click.echo("Installation complete ✅")


@main.command()
@click.option("--user", default="root", help="User to run the service as")
@click.option("--port", default=3000, help="WebSocket port")
def setup(user, port):
    """One-time setup: install browsers and setup systemd service"""
    click.echo("Starting one-time setup...")

    click.echo("Step 1: Installing browsers...")
    os.system(f"{sys.executable} -m playwright install webkit")
    os.system(f"{sys.executable} -m playwright install-deps webkit")

    click.echo("Step 2: Setting up systemd service...")
    executable = sys.executable
    service_content = f"""[Unit]
Description=Trakand Reach Playwright Orchestration Engine
After=network.target

[Service]
Type=simple
User={user}
WorkingDirectory={os.getcwd()}
ExecStart={executable} -m trakand_reach.cli run --port {port}
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
"""
    service_path = "/etc/systemd/system/trakand-reach.service"
    try:
        with open("/tmp/trakand-reach.service", "w") as f:
            f.write(service_content)

        os.system(f"sudo mv /tmp/trakand-reach.service {service_path}")
        os.system("sudo systemctl daemon-reload")
        os.system("sudo systemctl enable trakand-reach")
        click.echo(f"Successfully installed systemd service at {service_path} ✅")
        click.echo("Run 'sudo systemctl start trakand-reach' to start the service.")
    except Exception as e:
        click.echo(f"Could not write to systemd directory directly: {e}")
        click.echo("Generating local file 'trakand-reach.service' instead.")
        with open("trakand-reach.service", "w") as f:
            f.write(service_content)

    click.echo("Setup complete ✅")


@main.command()
@click.option("--port", default=3000, help="WebSocket port")
@click.option("--url", default="https://web.whatsapp.com", help="URL to open")
def whatsapp(port, url):
    """Quick start WhatsApp Web session"""
    click.echo(f"Starting WhatsApp Web session on port {port}...")

    async def _main() -> None:
        engine = PlaywrightService()
        device_info = {
            "userAgent": (
                "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 "
                "(KHTML, like Gecko) Version/17.0 Safari/605.1.15"
            ),
            "width": 1280,
            "height": 720,
            "pixelRatio": 1.0,
            "fingerprint": "whatsapp-default",
        }
        await engine.start()
        try:
            session = await engine.create_session("internal-key", device_info)
            async with websocket_serve(engine.handle_websocket, "0.0.0.0", port):
                click.echo(f"WebSocket: ws://0.0.0.0:{port}")
                click.echo("Navigate to the URL above to see the QR code.")
                await engine.start_up_link(session.id, url)
                await asyncio.Future()
        finally:
            await engine.stop()

    try:
        asyncio.run(_main())
    except KeyboardInterrupt:
        pass


@main.command()
@click.option("--port", default=3000, help="WebSocket port")
def bot(port):
    """Start a sample WhatsApp auto-reply bot"""
    click.echo(f"Starting Trakand Reach Bot on port {port}...")

    async def _main() -> None:
        engine = PlaywrightService()
        await engine.start()
        try:
            device_info = {
                "userAgent": (
                    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 "
                    "(KHTML, like Gecko) Version/17.0 Safari/605.1.15"
                ),
                "width": 1280,
                "height": 720,
                "pixelRatio": 1.0,
                "fingerprint": "bot-session",
            }
            session = await engine.create_session("bot-key", device_info)

            async def on_qr(data):
                click.echo(f"\n[QR CODE RECEIVED]\n{data}\n")

            async def on_message(data):
                text = data.get("text", "")
                sender = data.get("sender", "unknown")
                click.echo(f"New message from {sender}: {text}")

                if "hello" in text.lower():
                    click.echo(f"Auto-replying to {sender}...")
                    await engine.send_whatsapp_message(
                        session.id,
                        sender,
                        "Hello! I am a Trakand Reach Bot. How can I help you today?",
                    )

            session.event_listeners["qr"].append(on_qr)
            session.event_listeners["message_new"].append(on_message)

            async with websocket_serve(engine.handle_websocket, "0.0.0.0", port):
                await engine.start_up_link(session.id, "https://web.whatsapp.com")
                click.echo("Bot is running and listening for messages... ✅")
                await asyncio.Future()
        finally:
            await engine.stop()

    try:
        asyncio.run(_main())
    except KeyboardInterrupt:
        pass


if __name__ == "__main__":
    main()
