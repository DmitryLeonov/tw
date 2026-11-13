"""HTTP and WebSocket API client for the Territory Wars server."""

from __future__ import annotations

import json
import logging
import time
from typing import Any, Dict, Optional

import asyncio
import httpx
import websockets

from .models import (
    ActionMessage,
    Event,
    JoinResponse,
    Observation,
)

logger = logging.getLogger(__name__)


class GameApiClient:
    """Client for the Territory Wars HTTP + WebSocket API.

    Usage::

        client = GameApiClient(base_url="http://localhost:8080", token="abc...")
        try:
            obs = await client.get_observation()
            print(f"Turn {obs.turn}, Score {obs.score}")
        finally:
            await client.close()
    """

    def __init__(self, base_url: str, token: str, timeout: float = 10.0) -> None:
        self.base_url = base_url.rstrip("/")
        self.token = token
        self.timeout = timeout
        self._http_client = httpx.Client(timeout=timeout)
        self._ws = None

    # ------------------------------------------------------------------
    # Class methods
    # ------------------------------------------------------------------

    @staticmethod
    def register(base_url: str, name: str, timeout: float = 10.0) -> JoinResponse:
        """Register a new player and return the join response (class method)."""
        client = GameApiClient(base_url=base_url, token="", timeout=timeout)
        try:
            return client.join(name)
        finally:
            client._http_client.close()

    # ------------------------------------------------------------------
    # HTTP API
    # ------------------------------------------------------------------

    def join(self, name: str) -> JoinResponse:
        """Register a new player and return the join response."""
        payload = {"name": name}
        logger.info("Registering player: %s", name)
        resp = self._http_client.post(f"{self.base_url}/api/v1/join", json=payload)
        if resp.status_code in (200, 201):
            data = resp.json()
            return JoinResponse(
                player_id=data["player_id"],
                token=data["token"],
                websocket_url=data.get("websocket_url", "/ws"),
            )
        raise RuntimeError(f"join failed with status {resp.status_code}: {resp.text}")

    def get_observation(self) -> Observation:
        """Fetch the latest observation for the current player."""
        headers = {"Authorization": f"Bearer {self.token}"}
        resp = self._http_client.get(f"{self.base_url}/api/v1/state", headers=headers)
        if resp.status_code == 200:
            data = resp.json()
            return Observation.from_dict(data)
        raise RuntimeError(f"observation failed with status {resp.status_code}: {resp.text}")

    def get_leaderboard(self) -> Dict[str, Any]:
        """Fetch the current leaderboard."""
        resp = self._http_client.get(f"{self.base_url}/api/v1/leaderboard")
        if resp.status_code == 200:
            return resp.json()
        raise RuntimeError(f"leaderboard failed with status {resp.status_code}: {resp.text}")

    # ------------------------------------------------------------------
    # WebSocket
    # ------------------------------------------------------------------

    async def connect(self) -> websockets.WebSocketClientProtocol:
        """Open a WebSocket connection to the server.

        The connection URL is built from the base_url and the websocket_url
        returned during registration.
        """
        # Close any existing WebSocket before connecting (prevents stale connections).
        if self._ws is not None:
            try:
                await self._ws.close()
            except Exception:
                pass
            self._ws = None
        
        ws_url = self._build_ws_url()
        logger.info("Connecting to WebSocket %s", ws_url)
        ws = await websockets.connect(
            ws_url,
            ping_interval=20,
            ping_timeout=10,
            close_timeout=5,
        )
        self._ws = ws
        return ws

    async def submit_action(self, action_message: ActionMessage) -> bool:
        """Send an ActionMessage over the WebSocket.

        Returns True if the message was sent successfully.
        """
        if self._ws is None:
            logger.error("WebSocket not connected — call connect() first")
            return False
        payload = action_message.to_json()
        logger.debug("Sending action: %s", payload)
        try:
            await self._ws.send(payload)
            return True
        except websockets.exceptions.ConnectionClosed as exc:
            logger.error("WebSocket closed while sending: %s", exc)
            self._ws = None
            return False

    async def receive_event(self, timeout: Optional[float] = None) -> Optional[Event]:
        """Receive a single WebSocket event.

        Returns an Event dataclass, or None on timeout.
        """
        if self._ws is None:
            logger.error("WebSocket not connected")
            return None
        try:
            raw = await asyncio.wait_for(self._ws.recv(), timeout=timeout or 30.0)
            return Event.from_json(raw)
        except asyncio.TimeoutError:
            return None
        except websockets.exceptions.ConnectionClosed as exc:
            logger.warning("WebSocket closed: %s", exc)
            self._ws = None
            return None

    async def wait_for_event(self, timeout: float = 5.0) -> Optional[Event]:
        """Wait for a single WebSocket event with a short timeout.

        Used by the game loop to poll for events without blocking for too long.
        """
        if self._ws is None:
            return None
        try:
            raw = await asyncio.wait_for(self._ws.recv(), timeout=timeout)
            return Event.from_json(raw)
        except asyncio.TimeoutError:
            return None
        except websockets.exceptions.ConnectionClosed as exc:
            logger.warning("WebSocket closed: %s", exc)
            self._ws = None
            return None

    async def close(self) -> None:
        """Close the HTTP client and WebSocket connection."""
        if self._ws is not None:
            try:
                await self._ws.close()
            except Exception:
                pass
            self._ws = None
        self._http_client.close()
        logger.debug("API client closed")

    # ------------------------------------------------------------------
    # Helpers
    # ------------------------------------------------------------------

    def _build_ws_url(self) -> str:
        """Convert http://host:port to ws://host:port and append websocket path."""
        scheme = self.base_url.split("://")[0]
        ws_scheme = "ws" if scheme == "http" else "wss"
        host_port = self.base_url.split("://", 1)[1]
        ws_path = "/ws"
        return f"{ws_scheme}://{host_port}{ws_path}?token={self.token}"
