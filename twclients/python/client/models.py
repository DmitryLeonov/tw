"""Data models matching the Territory Wars Go server JSON schema."""

from __future__ import annotations

import json
from dataclasses import dataclass, field, asdict
from typing import Any, Dict, List, Optional


@dataclass
class Point:
    """A coordinate on the game map."""
    x: int
    y: int

    def to_dict(self) -> Dict[str, int]:
        return {"x": self.x, "y": self.y}

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "Point":
        return cls(x=data["x"], y=data["y"])

    def __eq__(self, other: object) -> bool:
        if not isinstance(other, Point):
            return NotImplemented
        return self.x == other.x and self.y == other.y

    def __hash__(self) -> int:
        return hash((self.x, self.y))


@dataclass
class Cell:
    """A single cell on the game map."""
    x: int
    y: int
    owner: str = ""
    health: int = 0
    type: str = "normal"

    def to_dict(self) -> Dict[str, Any]:
        result: Dict[str, Any] = {"x": self.x, "y": self.y, "health": self.health, "type": self.type}
        if self.owner:
            result["owner"] = self.owner
        return result

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "Cell":
        return cls(
            x=data["x"],
            y=data["y"],
            owner=data.get("owner", ""),
            health=data.get("health", 0),
            type=data.get("type", "normal"),
        )

    def __eq__(self, other: object) -> bool:
        if not isinstance(other, Cell):
            return NotImplemented
        return self.x == other.x and self.y == other.y

    def __hash__(self) -> int:
        return hash((self.x, self.y))


@dataclass
class PublicPlayer:
    """Public info about a player (no token, no name)."""
    player_id: str
    x: int
    y: int
    score: int = 0
    connected: bool = True
    eliminated: bool = False

    def to_dict(self) -> Dict[str, Any]:
        return asdict(self)

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "PublicPlayer":
        return cls(
            player_id=data["player_id"],
            x=data["x"],
            y=data["y"],
            score=data.get("score", 0),
            connected=data.get("connected", True),
            eliminated=data.get("eliminated", False),
        )


@dataclass
class Observation:
    """Full game state observation returned by GET /api/v1/state."""
    match_id: str
    player_id: str
    turn: int
    game_over: bool
    width: int
    height: int
    energy: int
    score: int
    cells: List[Cell]
    players: List[PublicPlayer]
    available_actions: List[str]

    def to_dict(self) -> Dict[str, Any]:
        return {
            "match_id": self.match_id,
            "player_id": self.player_id,
            "turn": self.turn,
            "game_over": self.game_over,
            "width": self.width,
            "height": self.height,
            "energy": self.energy,
            "score": self.score,
            "cells": [c.to_dict() for c in self.cells],
            "players": [p.to_dict() for p in self.players],
            "available_actions": self.available_actions,
        }

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "Observation":
        return cls(
            match_id=data["match_id"],
            player_id=data["player_id"],
            turn=data["turn"],
            game_over=data["game_over"],
            width=data["width"],
            height=data["height"],
            energy=data["energy"],
            score=data["score"],
            cells=[Cell.from_dict(c) for c in data["cells"]],
            players=[PublicPlayer.from_dict(p) for p in data["players"]],
            available_actions=data.get("available_actions", []),
        )

    @classmethod
    def from_json(cls, raw: str) -> "Observation":
        return cls.from_dict(json.loads(raw))

    def get_owned_cells(self) -> List[Cell]:
        """Return cells owned by this player."""
        return [c for c in self.cells if c.owner == self.player_id]

    def get_adjacent_cells(self, cell: Cell) -> List[Cell]:
        """Return cells adjacent (N/S/E/W) to the given cell."""
        neighbors = [
            (cell.x - 1, cell.y),
            (cell.x + 1, cell.y),
            (cell.x, cell.y - 1),
            (cell.x, cell.y + 1),
        ]
        cell_map = {(c.x, c.y): c for c in self.cells}
        return [cell_map[pt] for pt in neighbors if pt in cell_map]

    def is_valid_coord(self, x: int, y: int) -> bool:
        """Check whether (x, y) is within the map bounds."""
        return 0 <= x < self.width and 0 <= y < self.height


@dataclass
class Action:
    """A single action to submit (expand, attack, fortify, or pass)."""
    type: str
    target: Optional[Point] = None

    def to_dict(self) -> Dict[str, Any]:
        result: Dict[str, Any] = {"type": self.type}
        if self.target is not None:
            result["target"] = self.target.to_dict()
        return result

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "Action":
        target = None
        if "target" in data and data["target"] is not None:
            target = Point.from_dict(data["target"])
        return cls(type=data["type"], target=target)


@dataclass
class ActionMessage:
    """Envelope sent over WebSocket to submit actions."""
    type: str = "ACTION"
    turn: int = 0
    actions: List[Action] = field(default_factory=list)

    def to_dict(self) -> Dict[str, Any]:
        return {
            "type": self.type,
            "turn": self.turn,
            "actions": [a.to_dict() for a in self.actions],
        }

    def to_json(self) -> str:
        return json.dumps(self.to_dict())

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "ActionMessage":
        return cls(
            type=data.get("type", "ACTION"),
            turn=data.get("turn", 0),
            actions=[Action.from_dict(a) for a in data.get("actions", [])],
        )


@dataclass
class JoinResponse:
    """Response from POST /api/v1/join."""
    player_id: str
    token: str
    websocket_url: str = "/ws"

    def to_dict(self) -> Dict[str, Any]:
        return {
            "player_id": self.player_id,
            "token": self.token,
            "websocket_url": self.websocket_url,
        }

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "JoinResponse":
        return cls(
            player_id=data["player_id"],
            token=data["token"],
            websocket_url=data.get("websocket_url", "/ws"),
        )

    @classmethod
    def from_json(cls, raw: str) -> "JoinResponse":
        return cls.from_dict(json.loads(raw))


@dataclass
class Event:
    """WebSocket event received from the server."""
    type: str
    turn: int = 0
    message: str = ""
    observation: Optional[Observation] = None
    results: List[Dict[str, Any]] = field(default_factory=list)

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "Event":
        observation = None
        if "observation" in data and data["observation"] is not None:
            observation = Observation.from_dict(data["observation"])
        return cls(
            type=data["type"],
            turn=data.get("turn", 0),
            message=data.get("message", ""),
            observation=observation,
            results=data.get("results", []),
        )

    @classmethod
    def from_json(cls, raw: str) -> "Event":
        return cls.from_dict(json.loads(raw))
