using System.Text.Json.Serialization;

namespace TerritoryWars.Models;

public class Observation
{
    [JsonPropertyName("match_id")]
    public string MatchId { get; set; } = string.Empty;

    [JsonPropertyName("player_id")]
    public string PlayerId { get; set; } = string.Empty;

    public int Turn { get; set; }

    [JsonPropertyName("game_over")]
    public bool GameOver { get; set; }

    public int Width { get; set; }

    public int Height { get; set; }

    public int Energy { get; set; }

    public int Score { get; set; }

    public List<Cell> Cells { get; set; } = new();

    public List<PublicPlayer> Players { get; set; } = new();

    [JsonPropertyName("available_actions")]
    public List<string> AvailableActions { get; set; } = new();
}
