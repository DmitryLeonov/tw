"""Tests for SimpleStrategy."""

import pytest

from client.models import Action, Cell, Observation, Point, PublicPlayer
from client.strategy import SimpleStrategy


def make_observation(
    width: int = 40,
    height: int = 40,
    turn: int = 0,
    energy: int = 5,
    score: int = 0,
    owned_cells: list[dict] | None = None,
    neutral_cells: list[dict] | None = None,
    enemy_cells: list[dict] | None = None,
    player_id: str = "p01",
    extra_cells: list[dict] | None = None,
) -> Observation:
    """Helper to build a minimal Observation for testing.

    Only includes the explicitly provided cells — no default map fill.
    """
    cells: list[Cell] = []
    if owned_cells:
        for c in owned_cells:
            cells.append(Cell(x=c["x"], y=c["y"], owner=player_id, health=c.get("health", 3), type=c.get("type", "normal")))
    if neutral_cells:
        for c in neutral_cells:
            cells.append(Cell(x=c["x"], y=c["y"], owner="", health=c.get("health", 3), type=c.get("type", "normal")))
    if enemy_cells:
        for c in enemy_cells:
            cells.append(Cell(x=c["x"], y=c["y"], owner="p02", health=c.get("health", 3), type=c.get("type", "normal")))
    if extra_cells:
        for c in extra_cells:
            cells.append(Cell(x=c["x"], y=c["y"], owner=c.get("owner", ""), health=c.get("health", 3), type=c.get("type", "normal")))

    players = [
        PublicPlayer(player_id=player_id, x=0, y=0, score=score, connected=True),
    ]
    if enemy_cells:
        players.append(PublicPlayer(player_id="p02", x=10, y=10, score=0, connected=True))

    return Observation(
        match_id="test",
        player_id=player_id,
        turn=turn,
        game_over=False,
        width=width,
        height=height,
        energy=energy,
        score=score,
        cells=cells,
        players=players,
        available_actions=["expand", "attack", "fortify", "pass"],
    )


class TestSimpleStrategy:
    def setup_method(self):
        self.strategy = SimpleStrategy()

    def test_returns_valid_action_type(self):
        """Strategy returns one of the valid action types."""
        obs = make_observation(
            owned_cells=[{"x": 5, "y": 5, "health": 5, "type": "base"}],
            neutral_cells=[{"x": 6, "y": 5}],
        )
        action = self.strategy.decide(obs)
        assert action.type in ("expand", "attack", "fortify", "pass")

    def test_expands_into_adjacent_neutral(self):
        """Strategy expands into an adjacent neutral cell when possible."""
        obs = make_observation(
            owned_cells=[{"x": 5, "y": 5, "health": 5, "type": "base"}],
            neutral_cells=[{"x": 6, "y": 5}],
        )
        action = self.strategy.decide(obs)
        assert action.type == "expand"
        assert action.target is not None
        assert action.target.x == 6
        assert action.target.y == 5

    def test_handles_empty_cells_list(self):
        """Strategy returns pass when there are no owned cells."""
        obs = make_observation(owned_cells=[], neutral_cells=[])
        action = self.strategy.decide(obs)
        assert action.type == "pass"
        assert action.target is None

    def test_handles_insufficient_energy_for_expand(self):
        """Strategy passes when energy < 1 and no other action is possible."""
        obs = make_observation(
            owned_cells=[{"x": 5, "y": 5, "health": 5, "type": "base"}],
            neutral_cells=[],
            enemy_cells=[],
            energy=0,
        )
        action = self.strategy.decide(obs)
        assert action.type == "pass"

    def test_returns_pass_when_no_valid_action(self):
        """Strategy returns pass when surrounded by own cells with high health."""
        obs = make_observation(
            owned_cells=[
                {"x": 5, "y": 5, "health": 5, "type": "base"},
                {"x": 4, "y": 5, "health": 5, "type": "normal"},
                {"x": 6, "y": 5, "health": 5, "type": "normal"},
                {"x": 5, "y": 4, "health": 5, "type": "normal"},
                {"x": 5, "y": 6, "health": 5, "type": "normal"},
            ],
            neutral_cells=[],
            enemy_cells=[],
        )
        action = self.strategy.decide(obs)
        assert action.type == "pass"

    def test_coordinates_within_map_bounds(self):
        """All returned targets are within map bounds."""
        obs = make_observation(
            width=40,
            height=40,
            owned_cells=[{"x": 5, "y": 5, "health": 5, "type": "base"}],
            neutral_cells=[{"x": 6, "y": 5}],
        )
        action = self.strategy.decide(obs)
        if action.target is not None:
            assert 0 <= action.target.x < obs.width
            assert 0 <= action.target.y < obs.height

    def test_attacks_adjacent_enemy(self):
        """Strategy attacks an adjacent enemy cell when no neutral cells available."""
        obs = make_observation(
            owned_cells=[{"x": 5, "y": 5, "health": 5, "type": "base"}],
            enemy_cells=[{"x": 6, "y": 5, "health": 3, "type": "normal"}],
            neutral_cells=[],
        )
        action = self.strategy.decide(obs)
        assert action.type == "attack"
        assert action.target is not None
        assert action.target.x == 6
        assert action.target.y == 5

    def test_fortifies_low_health_cell(self):
        """Strategy fortifies a low-health owned cell when no expand/attack possible."""
        obs = make_observation(
            owned_cells=[
                {"x": 5, "y": 5, "health": 1, "type": "base"},
                {"x": 4, "y": 5, "health": 3, "type": "normal"},
                {"x": 6, "y": 5, "health": 3, "type": "normal"},
                {"x": 5, "y": 4, "health": 3, "type": "normal"},
                {"x": 5, "y": 6, "health": 3, "type": "normal"},
            ],
            neutral_cells=[],
            enemy_cells=[],
        )
        action = self.strategy.decide(obs)
        assert action.type == "fortify"
        assert action.target is not None
        assert action.target.x == 5
        assert action.target.y == 5

    def test_prefers_expand_over_attack(self):
        """Strategy prefers expand over attack when both are available."""
        obs = make_observation(
            owned_cells=[{"x": 5, "y": 5, "health": 5, "type": "base"}],
            neutral_cells=[{"x": 6, "y": 5}],
            enemy_cells=[{"x": 4, "y": 5, "health": 3, "type": "normal"}],
        )
        action = self.strategy.decide(obs)
        assert action.type == "expand"

    def test_passes_when_insufficient_energy_for_attack(self):
        """Strategy passes when energy < 2 and no expand possible."""
        obs = make_observation(
            owned_cells=[{"x": 5, "y": 5, "health": 5, "type": "base"}],
            enemy_cells=[{"x": 6, "y": 5, "health": 3, "type": "normal"}],
            neutral_cells=[],
            energy=1,
        )
        action = self.strategy.decide(obs)
        assert action.type == "pass"

    def test_passes_when_insufficient_energy_for_fortify(self):
        """Strategy passes when energy < 2 and no expand/attack possible."""
        obs = make_observation(
            owned_cells=[
                {"x": 5, "y": 5, "health": 1, "type": "base"},
                {"x": 4, "y": 5, "health": 3, "type": "normal"},
                {"x": 6, "y": 5, "health": 3, "type": "normal"},
                {"x": 5, "y": 4, "health": 3, "type": "normal"},
                {"x": 5, "y": 6, "health": 3, "type": "normal"},
            ],
            neutral_cells=[],
            enemy_cells=[],
            energy=1,
        )
        action = self.strategy.decide(obs)
        assert action.type == "pass"
