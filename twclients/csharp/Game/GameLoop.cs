using System.Diagnostics;
using TerritoryWars.Api;
using TerritoryWars.Models;
using TerritoryWars.Strategy;

namespace TerritoryWars.Game;

/// <summary>
/// The main game loop. Fetches observations, delegates to the strategy,
/// and submits actions each turn.
/// </summary>
public class GameLoop
{
    private readonly GameApiClient _apiClient;
    private readonly IGameStrategy _strategy;
    private readonly TimeSpan _pollInterval = TimeSpan.FromSeconds(2);
    private readonly Stopwatch _turnTimer = new();

    public GameLoop(GameApiClient apiClient, IGameStrategy strategy)
    {
        _apiClient = apiClient;
        _strategy = strategy;
    }

    /// <summary>
    /// Runs the game loop until the game ends or the cancellation token is triggered.
    /// </summary>
    public async Task RunAsync(CancellationToken ct)
    {
        Console.WriteLine("[GAME] Starting game loop...");

        while (!ct.IsCancellationRequested)
        {
            try
            {
                // Fetch current observation
                var observation = await _apiClient.GetObservationAsync(ct);

                // Check if game is over
                if (observation.GameOver)
                {
                    Console.WriteLine($"[GAME] Game over! Final score: {observation.Score}");
                    break;
                }

                Console.WriteLine(
                    $"[GAME] Turn {observation.Turn} | Score: {observation.Score} | Energy: {observation.Energy}");

                // Delegate to strategy to decide action
                var action = _strategy.Decide(observation);

                // Submit action via WebSocket
                if (action.Type != "pass")
                {
                    var message = new ActionMessage
                    {
                        Type = "ACTION",
                        Turn = observation.Turn,
                        Actions = new List<TerritoryWars.Models.Action> { action }
                    };

                    await _apiClient.SubmitActionAsync(message, ct);
                }
                else
                {
                    Console.WriteLine("[GAME] Strategy chose to pass.");
                }

                // Brief pause before next observation
                await Task.Delay(_pollInterval, ct);
            }
            catch (OperationCanceledException)
            {
                Console.WriteLine("[GAME] Game loop cancelled.");
                break;
            }
            catch (Exception ex)
            {
                Console.WriteLine($"[GAME] Error in game loop: {ex.Message}");

                // Wait before retrying
                await Task.Delay(3000, ct);
            }
        }

        Console.WriteLine("[GAME] Game loop finished.");
    }
}
