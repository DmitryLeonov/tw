using System.Text.Json.Serialization;

namespace TerritoryWars.Models;

public class ActionMessage
{
    public string Type { get; set; } = string.Empty;

    public int Turn { get; set; }

    public List<Action> Actions { get; set; } = new();
}
