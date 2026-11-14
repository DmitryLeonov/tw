using System.Diagnostics;
using TerritoryWars.Api;
using TerritoryWars.Models;
using TerritoryWars.Strategy;

namespace TerritoryWars.Game;

/// <summary>
/// The main game loop. Fetches observations, delegates to the strategy,
/// and submits actions each turn. Reconnects on WebSocket failure.
/// </summary>
public class GameLoop
{
    private readonly GameApiClient _apiClient;
    private readonly IGameStrategy _strategy;
    private readonly int _maxRetries;
    private readonly TimeSpan _pollInterval = TimeSpan.FromSeconds(0.5);

    public GameLoop(GameApiClient apiClient, IGameStrategy strategy, int maxRetries = 10)
    {
        _apiClient = apiClient;
        _strategy = strategy;
        _maxRetries = maxRetries;
    }

    /// <summary>
    /// Runs the game loop until the game ends or the cancellation token is triggered.
    /// </summary>
    public async Task RunAsync(CancellationToken ct)
    {
        Console.WriteLine("[GAME] Starting game loop...");

        int reconnects = 0;
        int maxReconnects = _maxRetries;

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

                // Wait for game to start (turn > 0) before sending actions.
                if (observation.Turn == 0)
                {
                    Console.WriteLine("[INFO] Waiting for game to start (turn 0)...");
                    await Task.Delay(1000, ct);
                    observation = await _apiClient.GetObservationAsync(ct);
                    if (observation.GameOver)
                    {
                        break;
                    }
                    continue;
                }

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
                    Console.WriteLine($"[INFO] Submitted action: {action.Type} → ({action.Target.X}, {action.Target.Y})");

                    // Wait for server response via WebSocket (ERROR or ACTION_ACCEPTED).
                    // This is critical: without it, we never detect rejected actions
                    // and keep retrying stale targets.
                    var response = await _apiClient.WaitForActionResponseAsync(observation.Turn, ct);

                    if (response.Rejected)
                    {
                        Console.WriteLine($"[GAME] Action rejected: {response.Reason}. Re-deciding...");
                        // Re-fetch observation to get fresh state
                        observation = await _apiClient.GetObservationAsync(ct);
                        // Continue loop to re-decide
                        continue;
                    }
                }
                else
                {
                    Console.WriteLine("[GAME] Strategy chose to pass.");
                }

                // Wait for turn resolution by polling observations.
                // Only proceed to the next turn once the turn number changes.
                int currentTurn = observation.Turn;
                while (observation.Turn == currentTurn && !observation.GameOver)
                {
                    await Task.Delay(_pollInterval, ct);
                    observation = await _apiClient.GetObservationAsync(ct);
                    if (observation.Turn != currentTurn)
                    {
                        Console.WriteLine(
                            $"[GAME] Turn resolved: {currentTurn} → {observation.Turn} | Score: {observation.Score}");
                    }
                }
                reconnects = 0;
            }
            catch (OperationCanceledException)
            {
                Console.WriteLine("[GAME] Game loop cancelled.");
                break;
            }
            catch (Exception ex)
            {
                Console.WriteLine($"[GAME] Error in game loop: {ex.Message}");
                reconnects++;

                // Check if it's a WebSocket connection error — try reconnecting
                if (ex.Message.Contains("WebSocket is not connected") ||
                    ex.Message.Contains("connection") ||
                    ex.Message.Contains("Connection"))
                {
                    Console.WriteLine("[GAME] WebSocket disconnected, reconnecting...");
                    if (reconnects >= maxReconnects)
                    {
                        Console.WriteLine($"[GAME] Max reconnects reached ({maxReconnects}), stopping.");
                        break;
                    }
                    try
                    {
                        await _apiClient.ConnectWebSocketAsync(ct);
                        // Re-fetch observation to get current state
                        var observation = await _apiClient.GetObservationAsync(ct);
                        Console.WriteLine($"[GAME] Reconnected at turn {observation.Turn}");
                        continue;
                    }
                    catch (Exception reconnectEx)
                    {
                        Console.WriteLine($"[GAME] Reconnect failed ({reconnectEx.Message}), trying re-join...");
                        // Re-join with a fresh token (happens after game reset).
                        try
                        {
                            var joinResp = await _apiClient.JoinAsync(_apiClient.PlayerName, ct);
                            _apiClient.UpdateToken(joinResp.Token);
                            Console.WriteLine($"[GAME] Re-joined game, player_id={joinResp.PlayerId}");
                            await _apiClient.ConnectWebSocketAsync(ct);
                            var observation = await _apiClient.GetObservationAsync(ct);
                            Console.WriteLine($"[GAME] Reconnected at turn {observation.Turn}");
                            continue;
                        }
                        catch (Exception joinEx)
                        {
                            var errStr = joinEx.Message;
                            if (errStr.Contains("registration is closed") ||
                                errStr.Contains("game stopped") ||
                                errStr.Contains("match is full"))
                            {
                                Console.WriteLine("[GAME] Registration closed — game cannot continue, exiting");
                                break;
                            }
                            Console.WriteLine($"[GAME] Re-join also failed: {joinEx.Message}");
                            await Task.Delay(5000, ct);
                            continue;
                        }
                    }
                }

                // Wait before retrying
                await Task.Delay(3000, ct);
            }
        }

        Console.WriteLine("[GAME] Game loop finished.");
    }
}
