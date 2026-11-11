using System.Text.Json.Serialization;

namespace TerritoryWars.Models;

public class PublicPlayer
{
    [JsonPropertyName("player_id")]
    public string PlayerId { get; set; } = string.Empty;

    public int X { get; set; }

    public int Y { get; set; }

    public int Score { get; set; }

    public bool Connected { get; set; }

    [JsonPropertyName("eliminated")]
    public bool Eliminated { get; set; }
}
