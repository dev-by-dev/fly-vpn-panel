package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	// Missing env file: defaults must apply, process env must be ignored.
	t.Setenv("FLYPANEL_ENV", filepath.Join(t.TempDir(), "missing.env"))
	t.Setenv("LISTEN_ADDR", "203.0.113.9:1")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	if cfg.DataDir != "./data" {
		t.Errorf("DataDir = %q, want %q", cfg.DataDir, "./data")
	}
	if cfg.ListenAddr != "127.0.0.1:3002" {
		t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, "127.0.0.1:3002")
	}
	if cfg.XrayBin != "./bin/xray" {
		t.Errorf("XrayBin = %q, want %q", cfg.XrayBin, "./bin/xray")
	}
	if cfg.PollIntervalSec != 30 {
		t.Errorf("PollIntervalSec = %d, want 30", cfg.PollIntervalSec)
	}
	if cfg.CookieSecure {
		t.Error("CookieSecure = true, want false by default")
	}
	if cfg.EntryIP != "" || cfg.EgressIP != "" || cfg.PanelDomain != "" {
		t.Errorf("IP/domain fields not empty: %+v", cfg)
	}
}

func TestLoadEnvFile(t *testing.T) {
	envPath := filepath.Join(t.TempDir(), ".env")
	content := "LISTEN_ADDR=203.0.113.1:3002\nEGRESS_IP=198.51.100.7\nCOOKIE_SECURE=true\n"
	if err := os.WriteFile(envPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	t.Setenv("FLYPANEL_ENV", envPath)
	// File value must win over process environment.
	t.Setenv("LISTEN_ADDR", "198.51.100.99:1")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	if cfg.ListenAddr != "203.0.113.1:3002" {
		t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, "203.0.113.1:3002")
	}
	if cfg.EgressIP != "198.51.100.7" {
		t.Errorf("EgressIP = %q, want %q", cfg.EgressIP, "198.51.100.7")
	}
	if !cfg.CookieSecure {
		t.Error("CookieSecure = false, want true from env file")
	}
	if cfg.DataDir != "./data" {
		t.Errorf("DataDir = %q, want default %q", cfg.DataDir, "./data")
	}
}
