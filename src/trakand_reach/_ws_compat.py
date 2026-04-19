"""Websocket server bootstrap compatible with websockets 10.x and 12+."""

from __future__ import annotations

from contextlib import asynccontextmanager
from typing import Any, AsyncIterator, Awaitable, Callable

try:
    from websockets.asyncio.server import serve as _ws_serve  # type: ignore[attr-defined]
except ImportError:  # websockets < 12
    from websockets import serve as _ws_serve  # type: ignore[no-redef]


@asynccontextmanager
async def websocket_serve(
    handler: Callable[..., Awaitable[Any]],
    host: str,
    port: int,
) -> AsyncIterator[Any]:
    async with _ws_serve(handler, host, port) as server:
        yield server
