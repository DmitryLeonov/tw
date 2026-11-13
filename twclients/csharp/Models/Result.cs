using System.Text.Json.Serialization;

namespace TerritoryWars.Models;

/// <summary>
/// Result of an action submission (returned in TURN_RESOLVED events).
/// </summary>
public class Result
{
    [JsonPropertyName("turn")]
    public int Turn { get; set; }

    [JsonPropertyName("player_id")]
    public string PlayerId { get; set; } = string.Empty;

    [JsonPropertyName("action")]
    public Action? Action { get; set; }

    [JsonPropertyName("accepted")]
    public bool Accepted { get; set; }

    [JsonPropertyName("reason")]
    public string Reason { get; set; } = string.Empty;
}
