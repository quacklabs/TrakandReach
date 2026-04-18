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
@click.option('--user', required=True, help='User to run the service as')
@click.option('--port', default=3000, help='WebSocket port')
def setup_service(user, port):
    """Generate and install a systemd service file"""
    executable = sys.executable
    # Use -m to run as a module which is more reliable for installed packages

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
        click.echo(f"Attempting to write service file...")

        with open("trakand-reach.service", "w") as f:
            f.write(service_content)

        click.echo("Successfully generated 'trakand-reach.service' in the current directory.")
        click.echo("\nTo install and start the service, run:")
        click.echo(f"  sudo mv trakand-reach.service {service_path}")
        click.echo("  sudo systemctl daemon-reload")
        click.echo("  sudo systemctl enable trakand-reach")
        click.echo("  sudo systemctl start trakand-reach")

    except Exception as e:
        click.echo(f"Error: {e}", err=True)

if __name__ == "__main__":
    main()
