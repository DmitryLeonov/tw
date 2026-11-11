"""Tests for API URL construction and model serialization."""

import json
import pytest

from client.api import GameApiClient
from client.models import Action, ActionMessage, Cell, Observation, Point


class TestUrlConstruction:
    """Test that WebSocket URLs are constructed correctly."""

    def test_http_to_ws(self):
        client = GameApiClient(base_url="http://localhost:8080", token="abc123")
        ws_url = client._build_ws_url()
        assert ws_url == "ws://localhost:8080/ws?token=abc123"

    def test_https_to_wss(self):
        client = GameApiClient(base_url="https://example.com", token="xyz")
        ws_url = client._build_ws_url()
        assert ws_url == "wss://example.com/ws?token=xyz"

    def test_trailing_slash_stripped(self):
        client = GameApiClient(base_url="http://localhost:8080/", token="abc")
        ws_url = client._build_ws_url()
        assert ws_url == "ws://localhost:8080/ws?token=abc"

    def test_port_preserved(self):
        client = GameApiClient(base_url="http://localhost:9090", token="token1")
        ws_url = client._build_ws_url()
        assert ws_url == "ws://localhost:9090/ws?token=token1"


class TestModelSerialization:
    """Test that models serialize/deserialize correctly for API payloads."""

    def test_action_message_serializes_correctly(self):
        """ActionMessage.to_dict() produces the exact JSON the server expects."""
        msg = ActionMessage(
            type="ACTION",
            turn=5,
            actions=[Action(type="expand", target=Point(x=10, y=20))],
        )
        d = msg.to_dict()
        assert d == {
            "type": "ACTION",
            "turn": 5,
            "actions": [{"type": "expand", "target": {"x": 10, "y": 20}}],
        }

    def test_action_message_json_roundtrip(self):
        """ActionMessage.to_json() → from_dict() roundtrip."""
        msg = ActionMessage(
            type="ACTION",
            turn=3,
            actions=[Action(type="attack", target=Point(x=7, y=12))],
        )
        json_str = msg.to_json()
        parsed = ActionMessage.from_dict(json.loads(json_str))
        assert parsed.type == "ACTION"
        assert parsed.turn == 3
        assert len(parsed.actions) == 1
        assert parsed.actions[0].type == "attack"
        assert parsed.actions[0].target.x == 7
        assert parsed.actions[0].target.y == 12

    def test_observation_serializes_correctly(self):
        """Observation.to_dict() includes all required fields."""
        obs = Observation(
            match_id="m1",
            player_id="p01",
            turn=0,
            game_over=False,
            width=40,
            height=40,
            energy=5,
            score=0,
            cells=[Cell(x=0, y=0, owner="p01", health=5, type="base")],
            players=[],
            available_actions=["expand", "attack", "fortify", "pass"],
        )
        d = obs.to_dict()
        assert d["match_id"] == "m1"
        assert d["player_id"] == "p01"
        assert d["turn"] == 0
        assert d["game_over"] is False
        assert d["width"] == 40
        assert d["height"] == 40
        assert d["energy"] == 5
        assert d["score"] == 0
        assert len(d["cells"]) == 1
        assert d["cells"][0]["owner"] == "p01"
        assert d["available_actions"] == ["expand", "attack", "fortify", "pass"]

    def test_observation_deserializes_server_response(self):
        """Observation.from_dict() handles the server's JSON format."""
        server_response = {
            "match_id": "abc123",
            "player_id": "p01",
            "turn": 1,
            "game_over": False,
            "width": 40,
            "height": 40,
            "energy": 4,
            "score": 1,
            "cells": [
                {"x": 0, "y": 0, "owner": "p01", "health": 5, "type": "base"},
                {"x": 1, "y": 0, "health": 3, "type": "normal"},
            ],
            "players": [
                {"player_id": "p01", "x": 0, "y": 0, "score": 1, "connected": True, "eliminated": False},
                {"player_id": "p02", "x": 39, "y": 39, "score": 0, "connected": True, "eliminated": False},
            ],
            "available_actions": ["expand", "attack", "fortify", "pass"],
        }
        obs = Observation.from_dict(server_response)
        assert obs.match_id == "abc123"
        assert obs.turn == 1
        assert obs.energy == 4
        assert obs.score == 1
        assert len(obs.cells) == 2
        assert len(obs.players) == 2
        assert obs.players[0].player_id == "p01"
        assert obs.players[1].player_id == "p02"

    def test_action_pass_serializes_without_target(self):
        """Pass action should not include a target field."""
        action = Action(type="pass")
        d = action.to_dict()
        assert d == {"type": "pass"}
        assert "target" not in d

    def test_action_with_target_serializes_correctly(self):
        """Action with target should include target as {x, y}."""
        action = Action(type="fortify", target=Point(x=3, y=4))
        d = action.to_dict()
        assert d == {"type": "fortify", "target": {"x": 3, "y": 4}}
