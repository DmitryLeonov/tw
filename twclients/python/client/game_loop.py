"""Game loop: connects, plays turns, and handles events."""

from __future__ import annotations

import asyncio
import logging
import time
from typing import Optional

from .api import GameApiClient
from .models import ActionMessage, Event, Observation
from .strategy import IGameStrategy

logger = logging.getLogger(__name__)


class GameLoop:
    """Orchestrates the game loop: connect → observe → decide → submit → repeat.

    Usage::

        loop = GameLoop(api_client, strategy)
        asyncio.run(loop.run())
    """

    def __init__(self, api_client: GameApiClient, strategy: IGameStrategy, max_retries: int = 10) -> None:
        self.api_client = api_client
        self.strategy = strategy
        self.max_retries = max_retries
        self._ws = None

    async def run(self) -> None:
        """Main game loop — simplified: connect → observe → decide → submit → repeat."""
        logger.info("Starting game loop")

        max_reconnects = self.max_retries
        reconnects = 0

        # --- Connect ---
        self._ws = await self.api_client.connect()
        logger.info("WebSocket connected")

        try:
            # --- Receive initial events until we get an OBSERVATION ---
            observation = await self._wait_for_observation()
            if observation is None:
                logger.error("Failed to receive initial observation")
                return

            logger.info(
                "Initial state: turn=%d score=%d energy=%d cells=%d",
                observation.turn,
                observation.score,
                observation.energy,
                len(observation.get_owned_cells()),
            )

            # --- Main loop ---
            while not observation.game_over:
                # Wait for game to start (turn > 0) before sending actions.
                if observation.turn == 0:
                    logger.info("Waiting for game to start (turn 0)...")
                    await asyncio.sleep(1)
                    observation = await self._wait_for_observation()
                    if observation is None:
                        return
                    continue

                # Decide action
                action = self.strategy.decide(observation)

                # Build and submit action message
                msg = ActionMessage(type="ACTION", turn=observation.turn, actions=[action])
                success = await self.api_client.submit_action(msg)
                if not success:
                    logger.error("Failed to submit action — reconnecting")
                    reconnects += 1
                    if reconnects >= max_reconnects:
                        logger.info("Max reconnects reached (%d), stopping", max_reconnects)
                        return
                    try:
                        self._ws = await self.api_client.connect()
                        observation = await self._wait_for_observation()
                        if observation is None:
                            return
                        reconnects = 0
                        continue
                    except Exception:
                        return

                logger.info(
                    "Turn %d: submitted %s action (target=%s)",
                    observation.turn,
                    action.type,
                    f"({action.target.x}, {action.target.y})" if action.target else "none",
                )

                # Wait for turn resolution and next observation
                observation = await self._wait_for_observation()
                if observation is None:
                    logger.warning("Lost connection or timeout — attempting recovery")
                    reconnects += 1
                    if reconnects >= max_reconnects:
                        logger.info("Max reconnects reached (%d), stopping", max_reconnects)
                        return
                    try:
                        self._ws = await self.api_client.connect()
                        observation = await self._wait_for_observation()
                        if observation is None:
                            return
                        reconnects = 0
                        continue
                    except Exception:
                        return

                logger.info(
                    "Turn %d complete: score=%d energy=%d game_over=%s",
                    observation.turn,
                    observation.score,
                    observation.energy,
                    observation.game_over,
                )

            # --- Game over ---
            self._print_final_state(observation)
            self._print_leaderboard()
            logger.info("Game over — final score: %d", observation.score)
        finally:
            await self.api_client.close()

    async def _wait_for_observation(self) -> Optional[Observation]:
        """Receive WebSocket events until an OBSERVATION is found.

        Returns the Observation, or None on error.
        """
        if self._ws is None:
            return None

        while True:
            event = await self.api_client.receive_event(timeout=10.0)
            if event is None:
                logger.warning("Timeout waiting for event")
                return None

            logger.debug("Received event type=%s turn=%d", event.type, event.turn)

            if event.type == "OBSERVATION" and event.observation is not None:
                return event.observation

            if event.type == "GAME_OVER":
                logger.info("Server announced game over: %s", event.message)
                # Try to get final observation
                try:
                    obs = await self.api_client.get_observation()
                    return obs
                except Exception:
                    return None

            if event.type == "ERROR":
                logger.error("Server error: %s", event.message)
                return None

            if event.type == "TURN_RESOLVED":
                # Continue waiting for OBSERVATION
                continue

            if event.type == "ACTION_ACCEPTED":
                # Continue waiting for TURN_RESOLVED + OBSERVATION
                continue

            if event.type == "STILL_ALIVE":
                # Server heartbeat — just keep waiting
                continue

            # CONNECTED and other events — skip
            continue

    def _print_final_state(self, observation: Observation) -> None:
        """Print the final game state."""
        print("\n" + "=" * 50)
        print("GAME OVER")
        print("=" * 50)
        print(f"Match ID:   {observation.match_id}")
        print(f"Your ID:    {observation.player_id}")
        print(f"Final Turn: {observation.turn}")
        print(f"Your Score: {observation.score}")
        print(f"Cells Owned: {len(observation.get_owned_cells())}")
        print("-" * 50)

        # Print all players
        for player in sorted(observation.players, key=lambda p: p.score, reverse=True):
            marker = " <-- YOU" if player.player_id == observation.player_id else ""
            status = "eliminated" if player.eliminated else ("connected" if player.connected else "disconnected")
            print(
                f"  {player.player_id}: score={player.score} "
                f"pos=({player.x},{player.y}) status={status}{marker}"
            )
        print("=" * 50 + "\n")

    def _print_leaderboard(self) -> None:
        """Print the current leaderboard from the HTTP API."""
        try:
            data = self.api_client.get_leaderboard()
            status = data.get("status", {})
            leaderboard = data.get("leaderboard", [])
            print("\nLeaderboard:")
            for rank, player in enumerate(leaderboard, 1):
                print(f"  {rank}. {player['player_id']}: score={player['score']}")
            print(f"\nGame status: {status}")
        except Exception as exc:
            logger.warning("Could not fetch leaderboard: %s", exc)
