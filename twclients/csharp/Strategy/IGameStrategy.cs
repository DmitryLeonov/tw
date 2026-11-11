using TerritoryWars.Models;

namespace TerritoryWars.Strategy;

/// <summary>
/// Interface for game strategies. Implement this interface to create
/// custom AI bots that decide actions each turn.
/// </summary>
public interface IGameStrategy
{
    /// <summary>
    /// Decides the action to take for the current observation.
    /// </summary>
    /// <param name="observation">The current game state.</param>
    /// <returns>An Action to submit, or "pass" if no action is available.</returns>
    TerritoryWars.Models.Action Decide(Observation observation);
}
