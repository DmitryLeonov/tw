using System.Net.WebSockets;
using System.Text;
using System.Text.Json;
using TerritoryWars.Models;

namespace TerritoryWars.Api;

public class GameApiClient : IDisposable
{
    private readonly HttpClient _httpClient;
    private readonly string _token;
    private readonly string _baseUrl;
    private ClientWebSocket? _webSocket;
    private bool _disposed;

    public GameApiClient(string baseUrl, string token, int timeoutSeconds)
    {
        _baseUrl = baseUrl.TrimEnd('/');
        _token = token;

        _httpClient = new HttpClient
        {
            Timeout = TimeSpan.FromSeconds(timeoutSeconds)
        };
        _httpClient.DefaultRequestHeaders.Authorization =
            new System.Net.Http.Headers.AuthenticationHeaderValue("Bearer", token);
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
    /// </summary>
    public async Task ConnectWebSocketAsync(CancellationToken ct)
    {
        var wsBase = _baseUrl.Replace("http://", "ws://").Replace("https://", "wss://");
        wsBase = wsBase.TrimEnd('/');
        var wsUri = $"{wsBase}/ws?token={_token}";
        _webSocket = new ClientWebSocket();
        await _webSocket.ConnectAsync(new Uri(wsUri), ct);
        Console.WriteLine("[INFO] WebSocket connected.");
    }

    /// <summary>
    /// Reads a message from the WebSocket.
    /// </summary>
    public async Task<string> ReadWebSocketMessageAsync(CancellationToken ct)
    {
        if (_webSocket == null || _webSocket.State != System.Net.WebSockets.WebSocketState.Open)
        {
            throw new InvalidOperationException(
                "WebSocket is not connected.");
        }

        var buffer = new byte[4096];
        var result = await _webSocket.ReceiveAsync(
            new ArraySegment<byte>(buffer), ct);

        var json = Encoding.UTF8.GetString(buffer, 0, result.Count);
        return json;
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
