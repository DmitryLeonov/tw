package strategy

import "twclient/internal/models"

// Strategy defines the interface that all game-playing strategies must implement.
// Decide returns the action the bot should take for the given observation.
// A nil action means "pass" (no target needed).
type Strategy interface {
	Decide(observation *models.Observation) *models.Action
}
