"""Strategy interface and built-in strategies for Territory Wars."""

from __future__ import annotations

import logging
from abc import ABC, abstractmethod
from typing import List, Optional, Tuple

from .models import Action, ActionMessage, Cell, Observation, Point

logger = logging.getLogger(__name__)


class IGameStrategy(ABC):
    """Abstract base class for game strategies.

    Subclasses implement ``decide`` to return an Action based on the
    current Observation.
    """

    @abstractmethod
    def decide(self, observation: Observation) -> Action:
        """Choose an action for the current turn.

        Args:
            observation: The current game state.

        Returns:
            An Action (expand, attack, fortify, or pass).
        """
        ...


class SimpleStrategy(IGameStrategy):
    """A simple heuristic strategy.

    Priority order:
    1. **Expand** into an adjacent neutral cell (costs 1 energy).
    2. **Attack** an adjacent enemy cell (costs 2 energy).
    3. **Fortify** an owned cell with low health (costs 2 energy).
    4. **Pass** if no valid action is available.
    """

    def decide(self, observation: Observation) -> Action:
        owned = observation.get_owned_cells()
        if not owned:
            logger.warning("No owned cells — passing")
            return Action(type="pass")

        # --- Expand: find adjacent neutral cells ---
        expand_target = self._find_expand_target(observation, owned)
        if expand_target is not None and observation.energy >= 1:
            logger.info("Expanding to (%d, %d)", expand_target.x, expand_target.y)
            return Action(type="expand", target=expand_target)

        # --- Attack: find adjacent enemy cells ---
        attack_target = self._find_attack_target(observation, owned)
        if attack_target is not None and observation.energy >= 2:
            logger.info("Attacking (%d, %d)", attack_target.x, attack_target.y)
            return Action(type="attack", target=attack_target)

        # --- Fortify: find owned cell with low health ---
        fortify_target = self._find_fortify_target(observation, owned)
        if fortify_target is not None and observation.energy >= 2:
            logger.info("Fortifying (%d, %d)", fortify_target.x, fortify_target.y)
            return Action(type="fortify", target=fortify_target)

        logger.info("No valid action — passing")
        return Action(type="pass")

    # ------------------------------------------------------------------
    # Internal helpers
    # ------------------------------------------------------------------

    def _find_expand_target(
        self, observation: Observation, owned: List[Cell]
    ) -> Optional[Point]:
        """Find an adjacent neutral cell to expand into."""
        cell_set = {(c.x, c.y) for c in owned}
        for cell in owned:
            for neighbor in self._neighbors(cell):
                if not observation.is_valid_coord(neighbor.x, neighbor.y):
                    continue
                if (neighbor.x, neighbor.y) in cell_set:
                    continue
                # Check if neutral (no owner)
                cell_data = next(
                    (c for c in observation.cells if c.x == neighbor.x and c.y == neighbor.y),
                    None,
                )
                if cell_data and cell_data.owner == "":
                    return neighbor
        return None

    def _find_attack_target(
        self, observation: Observation, owned: List[Cell]
    ) -> Optional[Point]:
        """Find an adjacent enemy cell to attack."""
        for cell in owned:
            for neighbor in self._neighbors(cell):
                if not observation.is_valid_coord(neighbor.x, neighbor.y):
                    continue
                cell_data = next(
                    (c for c in observation.cells if c.x == neighbor.x and c.y == neighbor.y),
                    None,
                )
                if cell_data and cell_data.owner != "" and cell_data.owner != observation.player_id:
                    return neighbor
        return None

    def _find_fortify_target(
        self, observation: Observation, owned: List[Cell]
    ) -> Optional[Point]:
        """Find an owned cell with health <= 3 to fortify."""
        candidates = [c for c in owned if c.health <= 3]
        if not candidates:
            return None
        # Pick the weakest cell
        weakest = min(candidates, key=lambda c: c.health)
        return Point(x=weakest.x, y=weakest.y)

    @staticmethod
    def _neighbors(cell: Cell) -> List[Point]:
        """Return the four cardinal neighbors of a cell."""
        return [
            Point(cell.x - 1, cell.y),
            Point(cell.x + 1, cell.y),
            Point(cell.x, cell.y - 1),
            Point(cell.x, cell.y + 1),
        ]


class AggressiveStrategy(IGameStrategy):
    """An aggressive strategy that prioritizes attacks over expansion."""

    def decide(self, observation: Observation) -> Action:
        owned = observation.get_owned_cells()
        if not owned:
            return Action(type="pass")

        # --- Attack first ---
        attack_target = self._find_attack_target(observation, owned)
        if attack_target is not None and observation.energy >= 2:
            logger.info("Aggressively attacking (%d, %d)", attack_target.x, attack_target.y)
            return Action(type="attack", target=attack_target)

        # --- Expand ---
        expand_target = self._find_expand_target(observation, owned)
        if expand_target is not None and observation.energy >= 1:
            logger.info("Expanding to (%d, %d)", expand_target.x, expand_target.y)
            return Action(type="expand", target=expand_target)

        # --- Fortify ---
        fortify_target = self._find_fortify_target(observation, owned)
        if fortify_target is not None and observation.energy >= 2:
            logger.info("Fortifying (%d, %d)", fortify_target.x, fortify_target.y)
            return Action(type="fortify", target=fortify_target)

        return Action(type="pass")

    def _find_attack_target(
        self, observation: Observation, owned: List[Cell]
    ) -> Optional[Point]:
        for cell in owned:
            for neighbor in SimpleStrategy._neighbors(cell):
                if not observation.is_valid_coord(neighbor.x, neighbor.y):
                    continue
                cell_data = next(
                    (c for c in observation.cells if c.x == neighbor.x and c.y == neighbor.y),
                    None,
                )
                if cell_data and cell_data.owner != "" and cell_data.owner != observation.player_id:
                    return neighbor
        return None

    def _find_expand_target(
        self, observation: Observation, owned: List[Cell]
    ) -> Optional[Point]:
        cell_set = {(c.x, c.y) for c in owned}
        for cell in owned:
            for neighbor in SimpleStrategy._neighbors(cell):
                if not observation.is_valid_coord(neighbor.x, neighbor.y):
                    continue
                if (neighbor.x, neighbor.y) in cell_set:
                    continue
                cell_data = next(
                    (c for c in observation.cells if c.x == neighbor.x and c.y == neighbor.y),
                    None,
                )
                if cell_data and cell_data.owner == "":
                    return neighbor
        return None

    def _find_fortify_target(
        self, observation: Observation, owned: List[Cell]
    ) -> Optional[Point]:
        candidates = [c for c in owned if c.health <= 3]
        if not candidates:
            return None
        weakest = min(candidates, key=lambda c: c.health)
        return Point(x=weakest.x, y=weakest.y)


def build_strategy(name: str) -> IGameStrategy:
    """Factory to build a strategy by name.

    Args:
        name: Strategy name (e.g. 'simple', 'aggressive').

    Returns:
        An IGameStrategy instance.

    Raises:
        ValueError: If the strategy name is unknown.
    """
    strategies = {
        "simple": SimpleStrategy,
        "aggressive": AggressiveStrategy,
    }
    cls = strategies.get(name.lower())
    if cls is None:
        raise ValueError(f"Unknown strategy: {name}. Available: {list(strategies.keys())}")
    return cls()
