#!/usr/bin/env python3
"""Territory Wars Python client — entry point.

Usage::

    python main.py --server http://localhost:8080 --name MyBot
    python main.py --config config.json
    python main.py --config config.json --log-level DEBUG
"""

from __future__ import annotations

import argparse
import asyncio
import json
import logging
import sys
from pathlib import Path
from typing import Any, Dict

from client.api import GameApiClient
from client.game_loop import GameLoop
from client.models import JoinResponse
from client.strategy import build_strategy

logger = logging.getLogger("territory_wars")


def load_config(path: str) -> Dict[str, Any]:
    """Load a JSON config file."""
    config_path = Path(path)
    if not config_path.exists():
        raise FileNotFoundError(f"Config file not found: {path}")
    with open(config_path, "r") as f:
        return json.load(f) or {}


def parse_args() -> argparse.Namespace:
    """Parse command-line arguments."""
    parser = argparse.ArgumentParser(
        description="Territory Wars Python Client",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""\
examples:
  python main.py --server http://localhost:8080 --name MyBot
  python main.py --config config.json
  python main.py --config config.json --log-level DEBUG
        """,
    )
    parser.add_argument(
        "--server",
        type=str,
        default=None,
        help="Server URL (e.g. http://localhost:8080). Overrides config.",
    )
    parser.add_argument(
        "--name",
        type=str,
        default=None,
        help="Player name. Overrides config.",
    )
    parser.add_argument(
        "--config",
        type=str,
        default=None,
        help="Path to JSON config file.",
    )
    parser.add_argument(
        "--log-level",
        type=str,
        default=None,
        choices=["DEBUG", "INFO", "WARNING", "ERROR"],
        help="Logging level. Overrides config.",
    )
    return parser.parse_args()


def build_config(args: argparse.Namespace) -> Dict[str, Any]:
    """Merge CLI args with config file defaults."""
    config: Dict[str, Any] = {
        "server": "http://localhost:8080",
        "player_name": "StudentBot",
        "http_timeout": 10,
        "log_level": "INFO",
        "strategy": "simple",
        "max_retries": 10,
    }

    if args.config:
        file_config = load_config(args.config)
        config.update(file_config)

    if args.server:
        config["server"] = args.server
    if args.name:
        config["player_name"] = args.name
    if args.log_level:
        config["log_level"] = args.log_level

    return config


def setup_logging(level: str) -> None:
    """Configure the logging system."""
    numeric_level = getattr(logging, level.upper(), logging.INFO)
    logging.basicConfig(
        level=numeric_level,
        format="%(asctime)s [%(levelname)s] %(name)s: %(message)s",
        datefmt="%H:%M:%S",
    )


async def run_game(server_url: str, player_name: str, http_timeout: float, strategy_name: str, max_retries: int = 10) -> int:
    """Register and run the async game loop."""
    # Register
    try:
        join: JoinResponse = GameApiClient.register(
            base_url=server_url,
            name=player_name,
            timeout=http_timeout,
        )
    except Exception as exc:
        logger.error("Registration failed: %s", exc)
        return 1

    # Create API client
    api_client = GameApiClient(
        base_url=server_url,
        token=join.token,
        timeout=http_timeout,
    )

    # Build strategy
    try:
        strategy = build_strategy(strategy_name)
    except ValueError as exc:
        logger.error("%s", exc)
        return 1

    # Run game loop
    loop = GameLoop(api_client=api_client, strategy=strategy, max_retries=max_retries)
    try:
        await loop.run()
    except KeyboardInterrupt:
        logger.info("Interrupted by user")
    except Exception as exc:
        logger.error("Game loop error: %s", exc, exc_info=True)
        return 1

    return 0


def main() -> int:
    """Entry point."""
    args = parse_args()
    config = build_config(args)

    setup_logging(config.get("log_level", "INFO"))
    logger.info("Territory Wars Python Client v1.0.0")
    logger.info("Config: %s", config)

    server_url = config["server"]
    player_name = config["player_name"]
    http_timeout = config.get("http_timeout", 10)
    strategy_name = config.get("strategy", "simple")
    max_retries = config.get("max_retries", 10)

    return asyncio.run(run_game(server_url, player_name, http_timeout, strategy_name, max_retries))


if __name__ == "__main__":
    sys.exit(main())
