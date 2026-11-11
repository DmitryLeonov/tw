using System.Net.WebSockets;
using System.Text;
using Microsoft.Extensions.Configuration;
using Microsoft.Extensions.Logging;
using TerritoryWars.Api;
using TerritoryWars.Game;
using TerritoryWars.Models;
using TerritoryWars.Strategy;

// Parse command-line arguments
var commandArgs = Environment.GetCommandLineArgs();
var configPath = "appsettings.json";
string? serverUrl = null;
string? playerName = null;
string strategyName = "Simple";

for (var i = 0; i < commandArgs.Length; i++)
{
    switch (commandArgs[i].ToLower())
    {
        case "--config":
            if (i + 1 < commandArgs.Length) configPath = commandArgs[++i];
            break;
        case "--server":
            if (i + 1 < commandArgs.Length) serverUrl = commandArgs[++i];
            break;
        case "--name":
            if (i + 1 < commandArgs.Length) playerName = commandArgs[++i];
            break;
        case "--strategy":
            if (i + 1 < commandArgs.Length) strategyName = commandArgs[++i];
            break;
    }
}

// Build configuration
var configuration = new ConfigurationBuilder()
    .AddJsonFile(configPath, optional: true, reloadOnChange: true)
    .AddEnvironmentVariables(prefix: "TERRITORYWARS_")
    .Build();

// Resolve settings: CLI args > config file > defaults
var baseUrl = serverUrl
              ?? configuration["ServerUrl"]
              ?? "http://localhost:8080";

var name = playerName
           ?? configuration["PlayerName"]
           ?? "StudentBot";

var timeoutSeconds = int.Parse(
    configuration["HttpTimeoutSeconds"]
    ?? "10");

var logLevel = configuration["LogLevel"]
               ?? "Information";

Console.WriteLine($"[CONFIG] Server: {baseUrl}");
Console.WriteLine($"[CONFIG] Player: {name}");
Console.WriteLine($"[CONFIG] Strategy: {strategyName}");
Console.WriteLine($"[CONFIG] Timeout: {timeoutSeconds}s");
Console.WriteLine($"[CONFIG] Log Level: {logLevel}");

// Create cancellation token for Ctrl+C handling
var cts = new CancellationTokenSource();
Console.CancelKeyPress += (sender, e) =>
{
    Console.WriteLine("\n[CTRL+C] Shutting down...");
    cts.Cancel();
    e.Cancel = true; // Prevent immediate termination to allow graceful shutdown
};

// Register strategy
IGameStrategy strategy = strategyName.ToLower() switch
{
    "simple" => new SimpleStrategy(),
    _ => throw new ArgumentException(
        $"Unknown strategy: {strategyName}. Supported: Simple")
};

// Register player with the server
Console.WriteLine("[AUTH] Registering player...");
using var httpClient = new HttpClient
{
    Timeout = TimeSpan.FromSeconds(timeoutSeconds)
};

var joinPayload = new { name = name };
var joinOptions = new System.Text.Json.JsonSerializerOptions
{
    PropertyNamingPolicy = System.Text.Json.JsonNamingPolicy.CamelCase
};
var joinJson = System.Text.Json.JsonSerializer.Serialize(joinPayload, joinOptions);
var joinContent = new System.Net.Http.StringContent(joinJson, Encoding.UTF8);
joinContent.Headers.ContentType.MediaType = "application/json";

var joinResponse = await httpClient.PostAsync(
    $"{baseUrl}/api/v1/join", joinContent, cts.Token);
joinResponse.EnsureSuccessStatusCode();

var joinJsonResult = await joinResponse.Content.ReadAsStringAsync(cts.Token);
var joinResultOptions = new System.Text.Json.JsonSerializerOptions
{
    PropertyNameCaseInsensitive = true
};
var joinResult = System.Text.Json.JsonSerializer.Deserialize<JoinResponse>(
    joinJsonResult, joinResultOptions)
    ?? throw new InvalidOperationException("Failed to parse join response.");

Console.WriteLine($"[AUTH] Registered as {joinResult.PlayerId}");

// Create API client with token
var apiClient = new GameApiClient(
    baseUrl,
    joinResult.Token,
    timeoutSeconds);

// Connect WebSocket
await apiClient.ConnectWebSocketAsync(cts.Token);

// Create and run game loop
var gameLoop = new GameLoop(apiClient, strategy);
await gameLoop.RunAsync(cts.Token);

// Cleanup
apiClient.Dispose();
httpClient.Dispose();

Console.WriteLine("[EXIT] Client shut down gracefully.");
