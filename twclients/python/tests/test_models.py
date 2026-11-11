"""Tests for model serialization and deserialization."""

import json
import pytest

from client.models import (
    Action,
    ActionMessage,
    Cell,
    Event,
    JoinResponse,
    Observation,
    Point,
    PublicPlayer,
)


# ------------------------------------------------------------------
# Point
# ------------------------------------------------------------------

class TestPoint:
    def test_to_dict(self):
        p = Point(x=5, y=10)
        assert p.to_dict() == {"x": 5, "y": 10}

    def test_from_dict(self):
        data = {"x": 3, "y": 7}
        p = Point.from_dict(data)
        assert p.x == 3
        assert p.y == 7

    def test_equality(self):
        assert Point(1, 2) == Point(1, 2)
        assert Point(1, 2) != Point(1, 3)

    def test_hash(self):
        s = {Point(1, 2), Point(3, 4)}
        assert Point(1, 2) in s


# ------------------------------------------------------------------
# Cell
# ------------------------------------------------------------------

class TestCell:
    def test_from_dict_neutral(self):
        data = {"x": 0, "y": 0, "health": 3, "type": "normal"}
        c = Cell.from_dict(data)
        assert c.x == 0
        assert c.y == 0
        assert c.owner == ""
        assert c.health == 3
        assert c.type == "normal"

    def test_from_dict_owned(self):
        data = {"x": 1, "y": 1, "owner": "p01", "health": 5, "type": "base"}
        c = Cell.from_dict(data)
        assert c.owner == "p01"
        assert c.health == 5
        assert c.type == "base"

    def test_to_dict_neutral(self):
        c = Cell(x=0, y=0, owner="", health=3, type="normal")
        d = c.to_dict()
        assert d == {"x": 0, "y": 0, "health": 3, "type": "normal"}
        assert "owner" not in d

    def test_to_dict_owned(self):
        c = Cell(x=1, y=1, owner="p01", health=5, type="base")
        d = c.to_dict()
        assert d["owner"] == "p01"
        assert d["health"] == 5
        assert d["type"] == "base"

    def test_equality(self):
        assert Cell(0, 0) == Cell(0, 0)
        assert Cell(0, 0) != Cell(0, 1)


# ------------------------------------------------------------------
# PublicPlayer
# ------------------------------------------------------------------

class TestPublicPlayer:
    def test_from_dict(self):
        data = {
            "player_id": "p01",
            "x": 20,
            "y": 20,
            "score": 5,
            "connected": True,
            "eliminated": False,
        }
        p = PublicPlayer.from_dict(data)
        assert p.player_id == "p01"
        assert p.x == 20
        assert p.y == 20
        assert p.score == 5
        assert p.connected is True
        assert p.eliminated is False

    def test_to_dict(self):
        p = PublicPlayer(player_id="p01", x=20, y=20, score=5)
        d = p.to_dict()
        assert d["player_id"] == "p01"
        assert d["score"] == 5


# ------------------------------------------------------------------
# Observation
# ------------------------------------------------------------------

class TestObservation:
    SAMPLE_JSON = json.dumps({
        "match_id": "abc123",
        "player_id": "p01",
        "turn": 0,
        "game_over": False,
        "width": 40,
        "height": 40,
        "energy": 5,
        "score": 0,
        "cells": [
            {"x": 0, "y": 0, "owner": "p01", "health": 5, "type": "base"},
            {"x": 1, "y": 0, "health": 3, "type": "normal"},
        ],
        "players": [
            {"player_id": "p01", "x": 0, "y": 0, "score": 0, "connected": True, "eliminated": False},
        ],
        "available_actions": ["expand", "attack", "fortify", "pass"],
    })

    def test_from_json(self):
        obs = Observation.from_json(self.SAMPLE_JSON)
        assert obs.match_id == "abc123"
        assert obs.player_id == "p01"
        assert obs.turn == 0
        assert obs.game_over is False
        assert obs.width == 40
        assert obs.height == 40
        assert obs.energy == 5
        assert obs.score == 0
        assert len(obs.cells) == 2
        assert len(obs.players) == 1
        assert obs.available_actions == ["expand", "attack", "fortify", "pass"]

    def test_to_dict(self):
        obs = Observation.from_json(self.SAMPLE_JSON)
        d = obs.to_dict()
        assert d["match_id"] == "abc123"
        assert d["turn"] == 0
        assert d["game_over"] is False
        assert len(d["cells"]) == 2
        assert len(d["players"]) == 1

    def test_get_owned_cells(self):
        obs = Observation.from_json(self.SAMPLE_JSON)
        owned = obs.get_owned_cells()
        assert len(owned) == 1
        assert owned[0].owner == "p01"

    def test_is_valid_coord(self):
        obs = Observation.from_json(self.SAMPLE_JSON)
        assert obs.is_valid_coord(0, 0) is True
        assert obs.is_valid_coord(39, 39) is True
        assert obs.is_valid_coord(-1, 0) is False
        assert obs.is_valid_coord(40, 0) is False
        assert obs.is_valid_coord(0, -1) is False
        assert obs.is_valid_coord(0, 40) is False

    def test_get_adjacent_cells(self):
        obs = Observation.from_json(self.SAMPLE_JSON)
        cell = obs.cells[0]  # (0, 0)
        adj = obs.get_adjacent_cells(cell)
        # (0,0) has neighbors (−1,0), (1,0), (0,−1), (0,1)
        # Only (1,0) is valid and in the cells list
        assert len(adj) == 1
        assert adj[0].x == 1
        assert adj[0].y == 0


# ------------------------------------------------------------------
# Action
# ------------------------------------------------------------------

class TestAction:
    def test_to_dict_with_target(self):
        a = Action(type="expand", target=Point(x=5, y=10))
        d = a.to_dict()
        assert d == {"type": "expand", "target": {"x": 5, "y": 10}}

    def test_to_dict_without_target(self):
        a = Action(type="pass")
        d = a.to_dict()
        assert d == {"type": "pass"}
        assert "target" not in d

    def test_from_dict_with_target(self):
        data = {"type": "attack", "target": {"x": 3, "y": 7}}
        a = Action.from_dict(data)
        assert a.type == "attack"
        assert a.target.x == 3
        assert a.target.y == 7

    def test_from_dict_without_target(self):
        data = {"type": "pass"}
        a = Action.from_dict(data)
        assert a.type == "pass"
        assert a.target is None


# ------------------------------------------------------------------
# ActionMessage
# ------------------------------------------------------------------

class TestActionMessage:
    def test_to_json(self):
        msg = ActionMessage(
            type="ACTION",
            turn=5,
            actions=[Action(type="expand", target=Point(x=10, y=20))],
        )
        d = msg.to_dict()
        assert d["type"] == "ACTION"
        assert d["turn"] == 5
        assert len(d["actions"]) == 1
        assert d["actions"][0]["type"] == "expand"
        assert d["actions"][0]["target"] == {"x": 10, "y": 20}

    def test_to_json_pass(self):
        msg = ActionMessage(type="ACTION", turn=3, actions=[Action(type="pass")])
        d = msg.to_dict()
        assert d["actions"][0] == {"type": "pass"}

    def test_from_dict(self):
        data = {
            "type": "ACTION",
            "turn": 7,
            "actions": [{"type": "fortify", "target": {"x": 1, "y": 1}}],
        }
        msg = ActionMessage.from_dict(data)
        assert msg.type == "ACTION"
        assert msg.turn == 7
        assert len(msg.actions) == 1
        assert msg.actions[0].type == "fortify"
        assert msg.actions[0].target.x == 1
        assert msg.actions[0].target.y == 1


# ------------------------------------------------------------------
# JoinResponse
# ------------------------------------------------------------------

class TestJoinResponse:
    def test_from_dict(self):
        data = {
            "player_id": "p01",
            "token": "abcdef123456",
            "websocket_url": "/ws",
        }
        j = JoinResponse.from_dict(data)
        assert j.player_id == "p01"
        assert j.token == "abcdef123456"
        assert j.websocket_url == "/ws"

    def test_to_dict(self):
        j = JoinResponse(player_id="p01", token="abc", websocket_url="/ws")
        d = j.to_dict()
        assert d["player_id"] == "p01"
        assert d["token"] == "abc"

    def test_from_json(self):
        raw = '{"player_id": "p02", "token": "xyz", "websocket_url": "/ws"}'
        j = JoinResponse.from_json(raw)
        assert j.player_id == "p02"
        assert j.token == "xyz"


# ------------------------------------------------------------------
# Event
# ------------------------------------------------------------------

class TestEvent:
    def test_from_dict_observation_event(self):
        data = {
            "type": "OBSERVATION",
            "turn": 3,
            "observation": {
                "match_id": "m1",
                "player_id": "p01",
                "turn": 3,
                "game_over": False,
                "width": 40,
                "height": 40,
                "energy": 5,
                "score": 1,
                "cells": [],
                "players": [],
                "available_actions": ["expand", "attack", "fortify", "pass"],
            },
        }
        event = Event.from_dict(data)
        assert event.type == "OBSERVATION"
        assert event.turn == 3
        assert event.observation is not None
        assert event.observation.match_id == "m1"

    def test_from_dict_error_event(self):
        data = {"type": "ERROR", "turn": 2, "message": "target is outside the map"}
        event = Event.from_dict(data)
        assert event.type == "ERROR"
        assert event.message == "target is outside the map"

    def test_from_dict_turn_resolved(self):
        data = {
            "type": "TURN_RESOLVED",
            "turn": 5,
            "results": [
                {"turn": 5, "player_id": "p01", "action": {"type": "expand"}, "accepted": True},
            ],
        }
        event = Event.from_dict(data)
        assert event.type == "TURN_RESOLVED"
        assert event.turn == 5
        assert len(event.results) == 1
