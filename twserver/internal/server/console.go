package server

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Console runs an interactive command loop reading from stdin.
// Supported commands: start, stop, pause, resume, quit, results.
func (s *Server) Console() {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("=== Territory Wars Console ===")
	fmt.Println("Commands: start, stop, pause, resume, results, quit")
	fmt.Println("====================================================")

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		switch strings.ToLower(line) {
		case "start":
			if s.StartGame() {
				fmt.Println("Game started.")
			} else {
				fmt.Println("Game already started.")
			}
		case "stop":
			fmt.Println("Resetting game...")
			s.rounds.cleanupAll()
			s.engine.Reset()
			s.ResetGameStarted()
			fmt.Println("Game reset to turn 0. Waiting for start.")
		case "pause":
			fmt.Println("Pausing game...")
			s.Pause()
			fmt.Println("Game paused.")
		case "resume":
			fmt.Println("Resuming game...")
			s.Resume()
			fmt.Println("Game resumed.")
		case "results":
			s.printResults()
		case "quit":
			fmt.Println("Shutting down...")
			s.Shutdown()
			os.Exit(0)
		default:
			fmt.Printf("Unknown command: %s (try: start, stop, pause, resume, results, quit)\n", line)
		}
	}
}

// printResults prints the leaderboard and game results to stdout and saves to a file.
func (s *Server) printResults() {
	lb := s.engine.Leaderboard()
	if len(lb) == 0 {
		fmt.Println("No players registered.")
		return
	}

	// Build the text output.
	var sb strings.Builder
	sb.WriteString("=== Game Results ===\n")
	sb.WriteString(fmt.Sprintf("%-4s %-20s %-8s %-8s %-10s\n", "Rank", "Name", "Score", "Alive", "Connected"))
	sb.WriteString(strings.Repeat("-", 55) + "\n")
	for i, p := range lb {
		alive := "yes"
		if p.Eliminated {
			alive = "no"
		}
		conn := "yes"
		if !p.Connected {
			conn = "no"
		}
		sb.WriteString(fmt.Sprintf("%-4d %-20s %-8d %-8s %-10s\n", i+1, p.Name, p.Score, alive, conn))
	}
	sb.WriteString("\n")
	text := sb.String()

	// Print to stdout.
	fmt.Print(text)

	// Save to file: results.yy.mm.dd.hhmm.txt
	ts := time.Now().Local().Format("060102.1504")
	filename := fmt.Sprintf("results.%s.txt", ts)
	dir := s.config.ResultsDir
	if dir == "" {
		dir = "results"
	}
	path := filepath.Join(dir, filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Printf("Warning: failed to create results dir %s: %v\n", dir, err)
		return
	}
	if err := os.WriteFile(path, []byte(text), 0644); err != nil {
		fmt.Printf("Warning: failed to save results to %s: %v\n", path, err)
		return
	}
	fmt.Printf("Results saved to %s\n", path)
}
