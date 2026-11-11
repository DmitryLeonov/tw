using System.Text.Json.Serialization;

namespace TerritoryWars.Models;

public class Action
{
    public string Type { get; set; } = string.Empty;

    public Point Target { get; set; } = new();
}

public class Point
{
    public int X { get; set; }

    public int Y { get; set; }
}
