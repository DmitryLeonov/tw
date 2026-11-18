package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server != "http://localhost:8080" || cfg.PlayerName != "StudentBot" {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
	if cfg.HTTPTimeout != 10 {
		t.Errorf("expected http_timeout default 10 seconds, got %d", cfg.HTTPTimeout)
	}
	if cfg.MaxRetries != 10 {
		t.Errorf("expected max_retries default 10, got %d", cfg.MaxRetries)
	}
}

func TestLoadParsesHTTPTimeoutAsSeconds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"http_timeout": 5, "max_retries": 3}`), 0o644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HTTPTimeout != 5 {
		t.Errorf("expected http_timeout 5, got %d", cfg.HTTPTimeout)
	}
	if cfg.MaxRetries != 3 {
		t.Errorf("expected max_retries 3, got %d", cfg.MaxRetries)
	}
}

func TestLoadReplacesNonPositiveValuesWithDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"http_timeout": 0, "max_retries": -1}`), 0o644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HTTPTimeout != 10 {
		t.Errorf("expected http_timeout default 10, got %d", cfg.HTTPTimeout)
	}
	if cfg.MaxRetries != 10 {
		t.Errorf("expected max_retries default 10, got %d", cfg.MaxRetries)
	}
}
