using System.Text.Json.Serialization;

namespace TerritoryWars.Models;

/// <summary>
/// Server event received via WebSocket (ERROR, ACTION_ACCEPTED, TURN_RESOLVED, OBSERVATION, etc.).
/// </summary>
public class ServerEvent
{
    [JsonPropertyName("type")]
    public string Type { get; set; } = string.Empty;

    [JsonPropertyName("turn")]
    public int Turn { get; set; }

    [JsonPropertyName("message")]
    public string Message { get; set; } = string.Empty;

    [JsonPropertyName("observation")]
    public Observation? Observation { get; set; }

    [JsonPropertyName("results")]
    public List<Result>? Results { get; set; }
}
