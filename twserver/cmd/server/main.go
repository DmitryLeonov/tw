package main

import (
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"twserver/internal/game"
	"twserver/internal/server"
)

// multiWriter writes to both stdout and a log file.
type multiWriter struct {
	mu   sync.Mutex
	out  io.Writer
	file io.Writer
}

func (w *multiWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil {
		if _, ferr := w.file.Write(p); ferr != nil {
			return 0, ferr
		}
	}
	return w.out.Write(p)
}

func main() {
	// Command-line flags.
	configPath := flag.String("config", "", "Path to JSON config file (optional)")
	flag.Parse()
	flag.CommandLine.Usage = func() {} // Suppress default usage

	port := env("PORT", "8080")
	players := intEnv("MAX_PLAYERS", 20)
	maxTurns := intEnv("MAX_TURNS", 200)
	turnTimeout := time.Duration(intEnv("TURN_TIMEOUT_SECONDS", 15)) * time.Second

	var engine *game.Engine
	var srv *server.Server
	var mw *multiWriter

	if *configPath != "" {
		// Load JSON config.
		cfg, err := server.LoadConfig(*configPath)
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}
		log.Printf("Loaded config from %s", *configPath)
		log.Printf("MarkRate=%d MarkThreshold=%d", cfg.MarkRate, cfg.MarkThreshold)

		// Set up multi-writer for stdout + log file from config.
		mw = &multiWriter{out: os.Stdout}
		if cfg.LogsDir != "" {
			logPath := filepath.Join(cfg.LogsDir, "server.log")
			f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				log.Fatalf("Failed to open log file %s: %v", logPath, err)
			}
			mw.file = f
			log.Printf("Logging to %s", logPath)
		}
		log.SetOutput(mw)

		// Override with env vars if set.
		if p := env("PORT", ""); p != "" {
			cfg.Port = intEnv("PORT", cfg.Port)
		}
		if mp := env("MAX_PLAYERS", ""); mp != "" {
			cfg.MaxPlayers = intEnv("MAX_PLAYERS", cfg.MaxPlayers)
		}
		if mt := env("MAX_TURNS", ""); mt != "" {
			cfg.MaxTurns = intEnv("MAX_TURNS", cfg.MaxTurns)
		}
		if tt := env("TURN_TIMEOUT_SECONDS", ""); tt != "" {
			turnTimeout = time.Duration(intEnv("TURN_TIMEOUT_SECONDS", 15)) * time.Second
		} else if t := cfg.TurnDelay; t != "" {
			if d, err := time.ParseDuration(t); err == nil {
				turnTimeout = d
			}
		}

		engine = game.NewEngine(game.Config{
			MaxPlayers:    cfg.MaxPlayers,
			MaxTurns:      cfg.MaxTurns,
			TurnDelay:     turnTimeout,
			Width:         cfg.Width,
			Height:        cfg.Height,
			MarkRate:      cfg.MarkRate,
			MarkThreshold: cfg.MarkThreshold,
		})

		srv = server.NewWithConfig(engine, cfg)
	} else {
		// Use default config (no bots).
		engine = game.NewEngine(game.Config{
			MaxPlayers:    players,
			MaxTurns:      maxTurns,
			TurnDelay:     turnTimeout,
			Width:         40,
			Height:        40,
			MarkRate:      10,
			MarkThreshold: 5,
		})
		srv = server.New(engine)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", srv.Health)
	mux.HandleFunc("/api/v1/join", srv.Join)
	mux.HandleFunc("/api/v1/state", srv.State)
	mux.HandleFunc("/api/v1/leaderboard", srv.Leaderboard)
	mux.HandleFunc("/api/v1/results", srv.Results)
	mux.HandleFunc("/api/v1/start", srv.Start)
	mux.HandleFunc("/api/v1/stop", srv.Stop)
	mux.HandleFunc("/api/v1/pause", srv.PauseHTTP)
	mux.HandleFunc("/api/v1/resume", srv.ResumeHTTP)
	mux.HandleFunc("/ws", srv.WebSocket)

	// Viewer routes.
	if srv != nil {
		viewer := srv.GetViewer()
		if viewer != nil {
			mux.HandleFunc("/", viewer.ServeViewerHandler)
			mux.HandleFunc("/viewer", viewer.ServeViewerHandler)
			mux.HandleFunc("/api/v1/viewer/", viewer.ServeViewerHandler)
		}
	}

	addr := ":" + port
	log.Printf("Territory Wars server listening on %s", addr)
	log.Printf("WebSocket: ws://localhost:%s/ws?token=PLAYER_TOKEN", port)
	if viewer := srv.GetViewer(); viewer != nil {
		log.Printf("Viewer: http://localhost:%s/viewer", port)
	}

	httpServer := &http.Server{
		Addr:    addr,
		Handler: withCORS(mux),
	}

	// Ignore SIGINT/SIGTERM so the server survives harness process-group kills.
	// The server can still be stopped via /api/v1/stop or console "quit" command.
	signal.Ignore(syscall.SIGINT, syscall.SIGTERM)

	// Interactive console.
	go srv.Console()

	log.Fatal(httpServer.ListenAndServe())
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func intEnv(key string, fallback int) int {
	v, err := strconv.Atoi(os.Getenv(key))
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
