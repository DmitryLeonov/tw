using System.Text.Json.Serialization;

namespace TerritoryWars.Models;

public class JoinResponse
{
    [JsonPropertyName("player_id")]
    public string PlayerId { get; set; } = string.Empty;

    public string Token { get; set; } = string.Empty;

    [JsonPropertyName("websocket_url")]
    public string WebsocketUrl { get; set; } = string.Empty;
}
