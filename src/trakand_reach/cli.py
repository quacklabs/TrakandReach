import click
import asyncio
import signal
import sys
import os
import logging
from .engine import PlaywrightService
import websockets

logger = logging.getLogger("trakand_reach.cli")

@click.group()
def main():
    """Trakand Reach CLI"""
    pass

async def shutdown(sig, loop, engine):
    """Cleanup tasks on signal reception"""
    logger.info(f"Received exit signal {sig.name}...")
    await engine.stop()
    tasks = [t for t in asyncio.all_tasks() if t is not asyncio.current_task()]
    [t.cancel() for t in tasks]
    await asyncio.gather(*tasks, return_exceptions=True)
    loop.stop()

@main.command()
@click.option('--port', default=3000, help='WebSocket port')
@click.option('--host', default='0.0.0.0', help='Host to bind to')
def run(port, host):
    """Run Trakand Reach in lightweight standalone mode"""
    engine = PlaywrightService()
    loop = asyncio.get_event_loop()

    # Handle signals for graceful shutdown
    signals = (signal.SIGHUP, signal.SIGTERM, signal.SIGINT)
    for s in signals:
        loop.add_signal_handler(
            s, lambda s=s: asyncio.create_task(shutdown(s, loop, engine))
        )

    async def start_standalone():
        await engine.start()
        async with websockets.serve(engine.handle_websocket, host, port):
            click.echo(f"Trakand Reach Standalone Service started ✅")
            click.echo(f"WebSocket: ws://{host}:{port}")
            await asyncio.Future()  # run forever

    try:
        loop.run_until_complete(start_standalone())
    except asyncio.CancelledError:
        pass
    finally:
        loop.close()
        click.echo("Service stopped.")

@main.command()
def install():
    """Install Playwright browsers and dependencies"""
    click.echo("Installing Playwright browsers (WebKit)...")
    os.system(f"{sys.executable} -m playwright install webkit")
    os.system(f"{sys.executable} -m playwright install-deps webkit")
    click.echo("Installation complete ✅")

@main.command()
@click.option('--user', default='root', help='User to run the service as')
@click.option('--port', default=3000, help='WebSocket port')
def setup(user, port):
    """One-time setup: install browsers and setup systemd service"""
    click.echo("Starting one-time setup...")

    # 1. Install browsers
    click.echo("Step 1: Installing browsers...")
    os.system(f"{sys.executable} -m playwright install webkit")
    os.system(f"{sys.executable} -m playwright install-deps webkit")

    # 2. Setup systemd
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
@click.option('--port', default=3000, help='WebSocket port')
@click.option('--url', default='https://web.whatsapp.com', help='URL to open')
def whatsapp(port, url):
    """Quick start WhatsApp Web session"""
    click.echo(f"Starting WhatsApp Web session on port {port}...")
    engine = PlaywrightService()
    loop = asyncio.get_event_loop()

    async def run_whatsapp():
        await engine.start()
        # Create a default device info for WhatsApp
        device_info = {
            "userAgent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15",
            "width": 1280,
            "height": 720,
            "pixelRatio": 1.0,
            "fingerprint": "whatsapp-default"
        }
        session = await engine.create_session("internal-key", device_info)

        async with websockets.serve(engine.handle_websocket, "0.0.0.0", port):
            click.echo(f"WebSocket: ws://0.0.0.0:{port}")
            click.echo(f"Navigate to the URL above to see the QR code.")
            await engine.start_up_link(session.id, url)
            await asyncio.Future()

    loop.run_until_complete(run_whatsapp())

if __name__ == "__main__":
    main()
