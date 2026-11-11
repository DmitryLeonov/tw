using System.Text.Json.Serialization;

namespace TerritoryWars.Models;

public class Cell
{
    public int X { get; set; }

    public int Y { get; set; }

    public string Owner { get; set; } = string.Empty;

    public int Health { get; set; }

    public string Type { get; set; } = string.Empty;
}
