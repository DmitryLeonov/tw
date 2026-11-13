using System.Collections.Concurrent;
using System.Net.WebSockets;
using System.Text;
using System.Text.Json;
using TerritoryWars.Models;

namespace TerritoryWars.Api;

public class GameApiClient : IDisposable
{
    private readonly HttpClient _httpClient;
    private string _token;
    private readonly string _baseUrl;
    private readonly string _playerName;
    private ClientWebSocket? _webSocket;
    private readonly ConcurrentQueue<string> _messageQueue = new();
    private Task? _readLoopTask;
    private CancellationTokenSource? _readLoopCts;
    private bool _disposed;

    public GameApiClient(string baseUrl, string token, int timeoutSeconds)
    {
        _baseUrl = baseUrl.TrimEnd('/');
        _token = token;
        _playerName = "Player";

        _httpClient = new HttpClient
        {
            Timeout = TimeSpan.FromSeconds(timeoutSeconds)
        };
        _httpClient.DefaultRequestHeaders.Authorization =
            new System.Net.Http.Headers.AuthenticationHeaderValue("Bearer", token);
    }

    /// <summary>
    /// Player name used for re-joining after a game reset.
    /// </summary>
    public string PlayerName => _playerName;

    /// <summary>
    /// Updates the bearer token and refreshes the Authorization header.
    /// Called after re-joining the game with a fresh token.
    /// </summary>
    public void UpdateToken(string newToken)
    {
        _token = newToken;
        _httpClient.DefaultRequestHeaders.Authorization =
            new System.Net.Http.Headers.AuthenticationHeaderValue("Bearer", newToken);
    }

    /// <summary>
    /// Fetches the current game state observation from the server.
    /// </summary>
    public async Task<Observation> GetObservationAsync(CancellationToken ct)
    {
        try
        {
            var response = await _httpClient.GetAsync(
                $"{_baseUrl}/api/v1/state", ct);

            response.EnsureSuccessStatusCode();

            var json = await response.Content.ReadAsStringAsync(ct);
            var options = new JsonSerializerOptions
            {
                PropertyNameCaseInsensitive = true
            };
            return JsonSerializer.Deserialize<Observation>(json, options)
                   ?? throw new InvalidOperationException(
                       "Server returned null observation.");
        }
        catch (HttpRequestException ex)
        {
            Console.WriteLine($"[ERROR] HTTP request failed: {ex.Message}");
            throw;
        }
        catch (TaskCanceledException)
        {
            Console.WriteLine("[ERROR] Request timed out.");
            throw;
        }
    }

    /// <summary>
    /// Submits an action message to the server via WebSocket.
    /// </summary>
    public async Task SubmitActionAsync(ActionMessage message, CancellationToken ct)
    {
        if (_webSocket == null || _webSocket.State != System.Net.WebSockets.WebSocketState.Open)
        {
            throw new InvalidOperationException(
                "WebSocket is not connected. Call ConnectWebSocketAsync first.");
        }

        var options = new JsonSerializerOptions
        {
            PropertyNamingPolicy = JsonNamingPolicy.CamelCase
        };
        var json = JsonSerializer.Serialize(message, options);
        var bytes = Encoding.UTF8.GetBytes(json);

        var segment = new ArraySegment<byte>(bytes);
        await _webSocket.SendAsync(segment, WebSocketMessageType.Text,
            true, ct);

        Console.WriteLine($"[INFO] Submitted action: {json}");
    }

    /// <summary>
    /// Fetches the current leaderboard from the server.
    /// </summary>
    public async Task<Dictionary<string, object>> GetLeaderboardAsync(
        CancellationToken ct)
    {
        try
        {
            var response = await _httpClient.GetAsync(
                $"{_baseUrl}/api/v1/leaderboard", ct);

            response.EnsureSuccessStatusCode();

            var json = await response.Content.ReadAsStringAsync(ct);
            var options = new JsonSerializerOptions
            {
                PropertyNameCaseInsensitive = true
            };
            var result = JsonSerializer.Deserialize<
                Dictionary<string, object>>(json, options)
                ?? throw new InvalidOperationException(
                    "Server returned null leaderboard.");
            return result;
        }
        catch (HttpRequestException ex)
        {
            Console.WriteLine($"[ERROR] Leaderboard request failed: {ex.Message}");
            throw;
        }
        catch (TaskCanceledException)
        {
            Console.WriteLine("[ERROR] Leaderboard request timed out.");
            throw;
        }
    }

    /// <summary>
    /// Registers a player with the server and returns the join response.
    /// </summary>
    public async Task<JoinResponse> JoinAsync(string playerName,
        CancellationToken ct)
    {
        var payload = new { name = playerName };
        var options = new JsonSerializerOptions
        {
            PropertyNamingPolicy = JsonNamingPolicy.CamelCase
        };
        var json = JsonSerializer.Serialize(payload, options);
        var content = new StringContent(json, Encoding.UTF8,
            "application/json");

        var response = await _httpClient.PostAsync(
            $"{_baseUrl}/api/v1/join", content, ct);

        response.EnsureSuccessStatusCode();

        var responseJson = await response.Content.ReadAsStringAsync(ct);
        var joinOptions = new JsonSerializerOptions
        {
            PropertyNameCaseInsensitive = true
        };
        return JsonSerializer.Deserialize<JoinResponse>(responseJson,
            joinOptions)
               ?? throw new InvalidOperationException(
                   "Server returned null join response.");
    }

    /// <summary>
    /// Connects a WebSocket to the server for action submission.
    /// Also starts a background read loop to keep the connection alive
    /// (responds to server pings and buffers incoming messages).
    /// </summary>
    public async Task ConnectWebSocketAsync(CancellationToken ct)
    {
        // Stop old read loop if running
        if (_readLoopCts != null)
        {
            _readLoopCts.Cancel();
            _readLoopCts.Dispose();
        }
        _readLoopTask?.Wait(1000);

        var wsBase = _baseUrl.Replace("http://", "ws://").Replace("https://", "wss://");
        wsBase = wsBase.TrimEnd('/');
        var wsUri = $"{wsBase}/ws?token={_token}";
        _webSocket = new ClientWebSocket();
        await _webSocket.ConnectAsync(new Uri(wsUri), ct);
        Console.WriteLine("[INFO] WebSocket connected.");

        // Start background read loop to handle pings and buffer messages.
        // ClientWebSocket auto-responds to pings, but only if Receive is called.
        _readLoopCts = new CancellationTokenSource();
        _readLoopTask = Task.Run(() => ReadLoopAsync(_readLoopCts.Token));
    }

    /// <summary>
    /// Background loop that continuously reads from the WebSocket.
    /// This keeps the connection alive (server pings are auto-answered)
    /// and buffers messages for WaitForActionResponseAsync.
    /// </summary>
    private async Task ReadLoopAsync(CancellationToken ct)
    {
        var buffer = new byte[65536];
        while (!ct.IsCancellationRequested && _webSocket != null)
        {
            if (_webSocket.State != System.Net.WebSockets.WebSocketState.Open)
                break;

            try
            {
                var result = await _webSocket.ReceiveAsync(
                    new ArraySegment<byte>(buffer), ct);

                if (result.CloseStatus != null)
                {
                    Console.WriteLine("[WS] Connection closed by server.");
                    break;
                }

                // Handle pings automatically (ClientWebSocket does this, but we need
                // to keep calling Receive to let it process them).
                if (result.MessageType == WebSocketMessageType.Close)
                    break;

                // Accumulate fragmented messages
                var sb = new StringBuilder();
                sb.Append(Encoding.UTF8.GetString(buffer, 0, result.Count));

                while (!result.EndOfMessage)
                {
                    result = await _webSocket.ReceiveAsync(
                        new ArraySegment<byte>(buffer), ct);
                    if (result.CloseStatus != null)
                    {
                        Console.WriteLine("[WS] Connection closed during fragment.");
                        return;
                    }
                    sb.Append(Encoding.UTF8.GetString(buffer, 0, result.Count));
                }

                var json = sb.ToString();
                if (!string.IsNullOrEmpty(json))
                {
                    _messageQueue.Enqueue(json);
                }
            }
            catch (OperationCanceledException)
            {
                break;
            }
            catch (Exception ex)
            {
                Console.WriteLine($"[WS] Read loop error: {ex.Message}");
                break;
            }
        }
    }

    /// <summary>
    /// Dequeues the next buffered message from the WebSocket.
    /// Returns null if no message is available.
    /// </summary>
    public string? DequeueWebSocketMessage()
    {
        if (_messageQueue.TryDequeue(out var msg))
            return msg;
        return null;
    }

    /// <summary>
    /// Checks if there are buffered messages available.
    /// </summary>
    public int BufferedMessageCount => _messageQueue.Count;

    /// <summary>
    /// Waits for the server to respond to the last submitted action.
    /// Reads from the background read loop's message queue.
    /// Times out after 10 seconds if no response is received.
    /// </summary>
    public async Task<ActionResponse> WaitForActionResponseAsync(int turn, CancellationToken ct)
    {
        var cts = CancellationTokenSource.CreateLinkedTokenSource(ct);
        cts.CancelAfter(TimeSpan.FromSeconds(10));

        var options = new JsonSerializerOptions
        {
            PropertyNameCaseInsensitive = true
        };

        try
        {
            while (true)
            {
                // Wait for a message in the queue
                var msg = await DequeueWithTimeoutAsync(cts.Token);
                if (msg == null)
                {
                    // Timeout
                    return new ActionResponse { Accepted = true };
                }

                var evt = JsonSerializer.Deserialize<ServerEvent>(msg, options);
                if (evt == null) continue;

                switch (evt.Type)
                {
                    case "ERROR":
                        Console.WriteLine($"[WS] ERROR (turn {evt.Turn}): {evt.Message}");
                        return new ActionResponse
                        {
                            Rejected = true,
                            Reason = evt.Message
                        };

                    case "ACTION_ACCEPTED":
                        Console.WriteLine($"[WS] Action accepted at turn {evt.Turn}");
                        return new ActionResponse { Accepted = true };

                    case "OBSERVATION":
                    case "TURN_RESOLVED":
                        // Turn-level events; keep waiting for action response.
                        // If the turn changed, the action was implicitly accepted.
                        if (evt.Turn != turn)
                        {
                            return new ActionResponse { Accepted = true };
                        }
                        break;

                    default:
                        // Ignore other events, keep waiting
                        break;
                }
            }
        }
        catch (OperationCanceledException)
        {
            return new ActionResponse { Rejected = true, Reason = "Timeout waiting for response" };
        }
    }

    /// <summary>
    /// Dequeues a message from the queue with a timeout.
    /// </summary>
    private async Task<string?> DequeueWithTimeoutAsync(CancellationToken ct)
    {
        // Try non-blocking dequeue first
        if (_messageQueue.TryDequeue(out var msg))
            return msg;

        // Poll the queue with a timeout
        var deadline = DateTime.UtcNow.AddSeconds(10);
        while (DateTime.UtcNow < deadline && !ct.IsCancellationRequested)
        {
            if (_messageQueue.TryDequeue(out var m))
                return m;
            await Task.Delay(50, ct);
        }
        return null; // Timeout
    }

    /// <summary>
    /// Disposes the WebSocket and HttpClient.
    /// </summary>
    public void Dispose()
    {
        if (_disposed) return;

        _webSocket?.CloseAsync(WebSocketCloseStatus.NormalClosure,
            "Client shutting down", CancellationToken.None).Wait();
        _webSocket?.Dispose();
        _httpClient.Dispose();

        _disposed = true;
    }
}

/// <summary>
/// Result of waiting for an action response from the server.
/// </summary>
public class ActionResponse
{
    public bool Accepted { get; set; }
    public bool Rejected { get; set; }
    public string Reason { get; set; } = string.Empty;
}
