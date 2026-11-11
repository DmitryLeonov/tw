using TerritoryWars.Models;
using TerritoryWars.Strategy;

namespace TerritoryWars.Tests;

public class StrategyTests
{
    private readonly SimpleStrategy _strategy = new();

    [Fact]
    public void Decide_ReturnsValidActionType()
    {
        // Arrange
        var observation = CreateObservation(
            cells: new[]
            {
                new Cell { X = 0, Y = 0, Owner = "p01", Health = 5, Type = "base" },
                new Cell { X = 1, Y = 0, Owner = "", Health = 5, Type = "normal" }
            },
            energy: 3);

        // Act
        var action = _strategy.Decide(observation);

        // Assert
        Assert.True(action.Type == "expand" || action.Type == "pass" ||
                    action.Type == "attack" || action.Type == "fortify",
            $"Unexpected action type: {action.Type}");
    }

    [Fact]
    public void Decide_HandlesEmptyCells()
    {
        // Arrange
        var observation = CreateObservation(
            cells: Array.Empty<Cell>(),
            energy: 5);

        // Act
        var action = _strategy.Decide(observation);

        // Assert
        Assert.Equal("pass", action.Type);
    }

    [Fact]
    public void Decide_HandlesInsufficientEnergy()
    {
        // Arrange
        var observation = CreateObservation(
            cells: new[]
            {
                new Cell { X = 0, Y = 0, Owner = "p01", Health = 5, Type = "base" },
                new Cell { X = 1, Y = 0, Owner = "", Health = 5, Type = "normal" }
            },
            energy: 0);

        // Act
        var action = _strategy.Decide(observation);

        // Assert
        Assert.Equal("pass", action.Type);
    }

    [Fact]
    public void Decide_ReturnsPassWhenNoValidAction()
    {
        // Arrange: Only one cell, no adjacent neutral or enemy cells
        var observation = CreateObservation(
            cells: new[]
            {
                new Cell { X = 0, Y = 0, Owner = "p01", Health = 5, Type = "base" }
            },
            energy: 5);

        // Act
        var action = _strategy.Decide(observation);

        // Assert
        Assert.Equal("pass", action.Type);
    }

    [Fact]
    public void Decide_CoordinatesWithinMapBounds()
    {
        // Arrange
        var width = 40;
        var height = 40;
        var observation = CreateObservation(
            width: width,
            height: height,
            cells: new[]
            {
                new Cell { X = 39, Y = 39, Owner = "p01", Health = 5, Type = "base" },
                new Cell { X = 39, Y = 38, Owner = "", Health = 5, Type = "normal" }
            },
            energy: 3);

        // Act
        var action = _strategy.Decide(observation);

        // Assert
        if (action.Type != "pass")
        {
            Assert.True(action.Target.X >= 0 && action.Target.X < width,
                $"X coordinate {action.Target.X} out of bounds [0, {width})");
            Assert.True(action.Target.Y >= 0 && action.Target.Y < height,
                $"Y coordinate {action.Target.Y} out of bounds [0, {height})");
        }
    }

    [Fact]
    public void Decide_PrioritizesAttackOverExpand()
    {
        // Arrange: Both enemy and neutral cells adjacent
        var observation = CreateObservation(
            cells: new[]
            {
                new Cell { X = 0, Y = 0, Owner = "p01", Health = 5, Type = "base" },
                new Cell { X = 1, Y = 0, Owner = "p02", Health = 3, Type = "normal" },
                new Cell { X = 0, Y = 1, Owner = "", Health = 5, Type = "normal" }
            },
            energy: 3);

        // Act
        var action = _strategy.Decide(observation);

        // Assert
        Assert.Equal("attack", action.Type);
        Assert.Equal(1, action.Target.X);
        Assert.Equal(0, action.Target.Y);
    }

    [Fact]
    public void Decide_ReturnsPassWhenGameOver()
    {
        // Arrange
        var observation = CreateObservation(
            cells: new[]
            {
                new Cell { X = 0, Y = 0, Owner = "p01", Health = 5, Type = "base" }
            },
            energy: 5,
            gameOver: true);

        // Act
        var action = _strategy.Decide(observation);

        // Assert
        Assert.Equal("pass", action.Type);
    }

    [Fact]
    public void Decide_ReturnsPassWhenPlayerEliminated()
    {
        // Arrange
        var observation = CreateObservation(
            cells: new[]
            {
                new Cell { X = 0, Y = 0, Owner = "p01", Health = 5, Type = "base" }
            },
            energy: 5,
            players: new[]
            {
                new PublicPlayer { PlayerId = "p01", Eliminated = true }
            });

        // Act
        var action = _strategy.Decide(observation);

        // Assert
        Assert.Equal("pass", action.Type);
    }

    [Fact]
    public void Decide_FortifiesWeakCell()
    {
        // Arrange: No adjacent neutral or enemy cells, but a weak cell to fortify
        var observation = CreateObservation(
            cells: new[]
            {
                new Cell { X = 0, Y = 0, Owner = "p01", Health = 2, Type = "normal" },
                new Cell { X = 1, Y = 0, Owner = "p01", Health = 5, Type = "normal" }
            },
            energy: 3);

        // Act
        var action = _strategy.Decide(observation);

        // Assert
        Assert.Equal("fortify", action.Type);
        Assert.Equal(0, action.Target.X);
        Assert.Equal(0, action.Target.Y);
    }

    // --- Helper Methods ---

    private static Observation CreateObservation(
        int width = 40,
        int height = 40,
        int energy = 5,
        bool gameOver = false,
        Cell[]? cells = null,
        PublicPlayer[]? players = null)
    {
        cells ??= Array.Empty<Cell>();
        players ??= new[]
        {
            new PublicPlayer
            {
                PlayerId = "p01",
                X = 0,
                Y = 0,
                Score = 0,
                Connected = true,
                Eliminated = false
            }
        };

        return new Observation
        {
            MatchId = "test-match-001",
            PlayerId = "p01",
            Turn = 0,
            GameOver = gameOver,
            Width = width,
            Height = height,
            Energy = energy,
            Score = 0,
            Cells = cells.ToList(),
            Players = players.ToList(),
            AvailableActions = new List<string> { "expand", "attack", "fortify", "pass" }
        };
    }
}
