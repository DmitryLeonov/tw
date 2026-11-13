package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_DefaultValues(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.json")
	if err := os.WriteFile(path, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Port)
	}
	if cfg.MaxPlayers != 20 {
		t.Errorf("expected max_players 20, got %d", cfg.MaxPlayers)
	}
	if cfg.MaxTurns != 200 {
		t.Errorf("expected max_turns 200, got %d", cfg.MaxTurns)
	}
}

func TestLoadConfig_OverrideValues(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.json")
	data := `{
		"port": 9090,
		"max_players": 10,
		"max_turns": 50,
		"bots_enabled": true
	}`
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Port)
	}
	if cfg.MaxPlayers != 10 {
		t.Errorf("expected max_players 10, got %d", cfg.MaxPlayers)
	}
	if cfg.MaxTurns != 50 {
		t.Errorf("expected max_turns 50, got %d", cfg.MaxTurns)
	}
	if !cfg.BotsEnabled {
		t.Errorf("expected bots_enabled true, got false")
	}
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.json")
	if err := os.WriteFile(path, []byte(`{invalid json:::`), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/config.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadConfig_NegativeValues(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.json")
	if err := os.WriteFile(path, []byte(`{"port": -1}`), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Port != 8080 {
		t.Errorf("expected port to default to 8080 for negative value, got %d", cfg.Port)
	}
}

func TestLoadConfig_ResultsDirRelative(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.json")
	if err := os.WriteFile(path, []byte(`{"results_dir": "results"}`), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	expected := filepath.Join(tmp, "results")
	if cfg.ResultsDir != expected {
		t.Errorf("expected results_dir %s, got %s", expected, cfg.ResultsDir)
	}
}

func TestLoadConfig_ResultsDirAbsolute(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.json")
	if err := os.WriteFile(path, []byte(`{"results_dir": "/tmp/my-results"}`), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.ResultsDir != "/tmp/my-results" {
		t.Errorf("expected absolute path unchanged, got %s", cfg.ResultsDir)
	}
}
