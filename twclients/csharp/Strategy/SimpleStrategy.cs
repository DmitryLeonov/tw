using TerritoryWars.Models;

namespace TerritoryWars.Strategy;

/// <summary>
/// A simple strategy that prioritizes expanding into neutral territory,
/// attacking adjacent enemies, and fortifying own cells when safe.
/// Falls back to pass when no valid action is available.
/// </summary>
public class SimpleStrategy : IGameStrategy
{
    public TerritoryWars.Models.Action Decide(Observation observation)
    {
        // If game is over or player is eliminated, pass
        if (observation.GameOver)
            return new TerritoryWars.Models.Action { Type = "pass" };

        var myPlayer = observation.Players.FirstOrDefault(
            p => p.PlayerId == observation.PlayerId);
        if (myPlayer == null || myPlayer.Eliminated)
            return new TerritoryWars.Models.Action { Type = "pass" };

        // If no energy available, pass
        if (observation.Energy < 1)
            return new TerritoryWars.Models.Action { Type = "pass" };

        // Get cells owned by the player
        var myCells = observation.Cells.Where(
            c => c.Owner == observation.PlayerId).ToList();

        if (!myCells.Any())
            return new TerritoryWars.Models.Action { Type = "pass" };

        // Find weakest cell health for defensive awareness.
        var weakestCell = myCells.OrderBy(c => c.Health).FirstOrDefault();
        if (weakestCell == null)
            return new TerritoryWars.Models.Action { Type = "pass" };
        var weakestHealth = weakestCell.Health;

        // Build a set of cells adjacent to our owned cells
        var adjacentCells = GetAdjacentCells(myCells, observation);

        // Defensive fallback: when weakest cell health <= 3, prioritize survival.
        // Fortify weakest cell > expand > pass. Attack is skipped to prevent
        // losing all territory when under pressure.
        if (weakestHealth <= 3)
        {
            // Priority 1: Fortify weakest cell (costs 2 energy).
            if (observation.Energy >= 2)
            {
                Console.WriteLine($"[STRATEGY] Fortifying weak cell at ({weakestCell.X}, {weakestCell.Y}) health={weakestHealth}");
                return new TerritoryWars.Models.Action
                {
                    Type = "fortify",
                    Target = new Point { X = weakestCell.X, Y = weakestCell.Y }
                };
            }
            // Priority 2: Expand to recover territory (costs 1 energy).
            var neutralCell = adjacentCells.FirstOrDefault(c =>
                c.Owner == string.Empty);
            if (neutralCell != null && observation.Energy >= 1)
            {
                Console.WriteLine($"[STRATEGY] Expanding to recover territory (weakest health={weakestHealth})");
                return new TerritoryWars.Models.Action
                {
                    Type = "expand",
                    Target = new Point { X = neutralCell.X, Y = neutralCell.Y }
                };
            }
            Console.WriteLine($"[STRATEGY] Passing (defensive: weakest health={weakestHealth})");
            return new TerritoryWars.Models.Action { Type = "pass" };
        }

        // Priority 1: Attack adjacent enemy cells
        var enemyCell = adjacentCells.FirstOrDefault(c =>
            c.Owner != string.Empty && c.Owner != observation.PlayerId);
        if (enemyCell != null)
        {
            Console.WriteLine($"[STRATEGY] Attacking enemy at ({enemyCell.X}, {enemyCell.Y})");
            return new TerritoryWars.Models.Action
            {
                Type = "attack",
                Target = new Point { X = enemyCell.X, Y = enemyCell.Y }
            };
        }

        // Priority 2: Expand into adjacent neutral cells
        var neutralCell2 = adjacentCells.FirstOrDefault(c =>
            c.Owner == string.Empty);
        if (neutralCell2 != null)
        {
            Console.WriteLine($"[STRATEGY] Expanding to neutral at ({neutralCell2.X}, {neutralCell2.Y})");
            return new TerritoryWars.Models.Action
            {
                Type = "expand",
                Target = new Point { X = neutralCell2.X, Y = neutralCell2.Y }
            };
        }

        // Priority 3: Fortify our weakest cell if health <= 3
        if (weakestCell != null && weakestCell.Health <= 3)
        {
            Console.WriteLine($"[STRATEGY] Fortifying our cell at ({weakestCell.X}, {weakestCell.Y})");
            return new TerritoryWars.Models.Action
            {
                Type = "fortify",
                Target = new Point { X = weakestCell.X, Y = weakestCell.Y }
            };
        }

        // No valid action available
        Console.WriteLine("[STRATEGY] Passing (no valid action)");
        return new TerritoryWars.Models.Action { Type = "pass" };
    }

    /// <summary>
    /// Finds all cells adjacent (Manhattan distance = 1) to the given cells.
    /// </summary>
    private static List<Cell> GetAdjacentCells(List<Cell> cells,
        Observation observation)
    {
        var adjacent = new List<Cell>();
        var cellSet = new HashSet<(int x, int y)>();

        foreach (var cell in cells)
        {
            cellSet.Add((cell.X, cell.Y));
        }

        foreach (var cell in observation.Cells)
        {
            if (cellSet.Contains((cell.X, cell.Y)))
            {
                // Check all 4 neighbors
                var neighbors = new[]
                {
                    (cell.X - 1, cell.Y),
                    (cell.X + 1, cell.Y),
                    (cell.X, cell.Y - 1),
                    (cell.X, cell.Y + 1)
                };

                foreach (var (rawX, rawY) in neighbors)
                {
                    // Wrap around edges (toroidal map)
                    var nx = (rawX + observation.Width) % observation.Width;
                    var ny = (rawY + observation.Height) % observation.Height;

                    // Find the cell at this position
                    var neighbor = observation.Cells.FirstOrDefault(
                        c => c.X == nx && c.Y == ny);
                    if (neighbor != null && !adjacent.Contains(neighbor))
                    {
                        adjacent.Add(neighbor);
                    }
                }
            }
        }

        return adjacent;
    }
}
