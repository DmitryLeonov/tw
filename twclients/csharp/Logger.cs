using System.Text;

namespace TerritoryWars;

/// <summary>
/// Shared dual writer for Console + optional log file.
/// Set LogFile path via Init() before use.
/// </summary>
public static class Logger
{
    private static readonly object _lock = new();
    private static StreamWriter? _fileWriter;

    public static void Init(string? logFile = null)
    {
        if (!string.IsNullOrWhiteSpace(logFile))
        {
            var dir = Path.GetDirectoryName(logFile);
            if (!string.IsNullOrWhiteSpace(dir))
                Directory.CreateDirectory(dir);

            _fileWriter = new StreamWriter(File.Open(logFile, FileMode.Append, FileAccess.Write, FileShare.Read))
            {
                AutoFlush = true
            };
        }
    }

    public static void WriteLine(string? value = null)
    {
        lock (_lock)
        {
            Console.WriteLine(value);
            _fileWriter?.WriteLine(value);
        }
    }

    public static void WriteLine(string format, params object?[] args)
    {
        var line = string.Format(format, args);
        lock (_lock)
        {
            Console.WriteLine(line);
            _fileWriter?.WriteLine(line);
        }
    }

    public static void Dispose()
    {
        lock (_lock)
        {
            _fileWriter?.Dispose();
            _fileWriter = null;
        }
    }
}
