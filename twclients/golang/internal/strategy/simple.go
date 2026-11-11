package strategy

import (
	"fmt"
	"log/slog"

	"twclient/internal/models"
)

// SimpleStrategy implements a basic heuristic-based decision loop.
//
// Priority order:
//   1. Attack an adjacent enemy cell (if energy >= 2 and enemy is weak).
//   2. Expand into an adjacent neutral cell (if energy >= 1).
//   3. Fortify an owned cell with low health (if energy >= 2).
//   4. Pass.
type SimpleStrategy struct {
	logger *slog.Logger
}

// NewSimpleStrategy creates a SimpleStrategy with the given logger.
func NewSimpleStrategy(logger *slog.Logger) *SimpleStrategy {
	if logger == nil {
		logger = slog.Default()
	}
	return &SimpleStrategy{logger: logger}
}

// Decide evaluates the observation and returns the best action.
func (s *SimpleStrategy) Decide(obs *models.Observation) *models.Action {
	if obs == nil {
		return nil
	}

	playerID := obs.PlayerID

	// Build lookup maps for quick access.
	cellsByCoord := make(map[string]*models.Cell, len(obs.Cells))
	for i := range obs.Cells {
		c := &obs.Cells[i]
		key := fmt.Sprintf("%d,%d", c.X, c.Y)
		cellsByCoord[key] = c
	}

	// Collect our owned cells and their coordinates.
	ownedCells := make([]*models.Cell, 0)
	for i := range obs.Cells {
		c := &obs.Cells[i]
		if c.Owner == playerID {
			ownedCells = append(ownedCells, c)
		}
	}

	// Find adjacent enemy cells (attack candidates).
	attackTargets := s.findAdjacentEnemyCells(obs, playerID, cellsByCoord)
	if len(attackTargets) > 0 && obs.Energy >= 2 {
		// Pick the weakest adjacent enemy cell to attack.
		target := attackTargets[0]
		for _, c := range attackTargets[1:] {
			if c.Health < target.Health {
				target = c
			}
		}
		s.logger.Info("deciding attack", "turn", obs.Turn, "x", target.X, "y", target.Y, "health", target.Health)
		return &models.Action{Type: "attack", Target: &models.Point{X: target.X, Y: target.Y}}
	}

	// Find adjacent neutral cells (expand candidates).
	expandTargets := s.findAdjacentNeutralCells(obs, playerID, cellsByCoord)
	if len(expandTargets) > 0 && obs.Energy >= 1 {
		target := expandTargets[0]
		s.logger.Info("deciding expand", "turn", obs.Turn, "x", target.X, "y", target.Y)
		return &models.Action{Type: "expand", Target: &models.Point{X: target.X, Y: target.Y}}
	}

	// Find owned cells with low health (fortify candidates).
	fortifyTargets := s.findLowHealthOwnedCells(ownedCells)
	if len(fortifyTargets) > 0 && obs.Energy >= 2 {
		target := fortifyTargets[0]
		s.logger.Info("deciding fortify", "turn", obs.Turn, "x", target.X, "y", target.Y, "health", target.Health)
		return &models.Action{Type: "fortify", Target: &models.Point{X: target.X, Y: target.Y}}
	}

	s.logger.Info("deciding pass", "turn", obs.Turn, "reason", "no valid action")
	return nil
}

// findAdjacentEnemyCells returns cells adjacent to any owned cell that belong
// to an enemy player.
func (s *SimpleStrategy) findAdjacentEnemyCells(obs *models.Observation, playerID string, cellsByCoord map[string]*models.Cell) []*models.Cell {
	ownedCells := make([]*models.Cell, 0)
	for i := range obs.Cells {
		c := &obs.Cells[i]
		if c.Owner == playerID {
			ownedCells = append(ownedCells, c)
		}
	}

	seen := make(map[string]bool)
	enemies := make([]*models.Cell, 0)

	for _, own := range ownedCells {
		for _, dir := range dirs {
			key := fmt.Sprintf("%d,%d", own.X+dir[0], own.Y+dir[1])
			if neighbor, ok := cellsByCoord[key]; ok {
				if neighbor.Owner != "" && neighbor.Owner != playerID && !seen[key] {
					seen[key] = true
					enemies = append(enemies, neighbor)
				}
			}
		}
	}

	return enemies
}

// findAdjacentNeutralCells returns cells adjacent to any owned cell that are
// neutral (no owner).
func (s *SimpleStrategy) findAdjacentNeutralCells(obs *models.Observation, playerID string, cellsByCoord map[string]*models.Cell) []*models.Cell {
	ownedCells := make([]*models.Cell, 0)
	for i := range obs.Cells {
		c := &obs.Cells[i]
		if c.Owner == playerID {
			ownedCells = append(ownedCells, c)
		}
	}

	seen := make(map[string]bool)
	neutrals := make([]*models.Cell, 0)

	for _, own := range ownedCells {
		for _, dir := range dirs {
			key := fmt.Sprintf("%d,%d", own.X+dir[0], own.Y+dir[1])
			if neighbor, ok := cellsByCoord[key]; ok {
				if neighbor.Owner == "" && !seen[key] {
					seen[key] = true
					neutrals = append(neutrals, neighbor)
				}
			}
		}
	}

	return neutrals
}

// findLowHealthOwnedCells returns owned cells with health <= 3, sorted by
// ascending health (most vulnerable first).
func (s *SimpleStrategy) findLowHealthOwnedCells(owned []*models.Cell) []*models.Cell {
	candidates := make([]*models.Cell, 0)
	for _, c := range owned {
		if c.Health <= 3 {
			candidates = append(candidates, c)
		}
	}
	// Sort by health ascending (simple insertion sort for small slices).
	for i := 1; i < len(candidates); i++ {
		for j := i; j > 0 && candidates[j].Health < candidates[j-1].Health; j-- {
			candidates[j], candidates[j-1] = candidates[j-1], candidates[j]
		}
	}
	return candidates
}

// dirs are the four cardinal directions (dx, dy).
var dirs = [][2]int{
	{-1, 0}, // left
	{1, 0},  // right
	{0, -1}, // up
	{0, 1},  // down
}
