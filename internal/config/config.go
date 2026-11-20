// Package config loads panel settings from an env file.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds panel settings. Values come from the env file selected by
// FLYPANEL_ENV (default ./.env); the process environment never overrides
// file values, and a missing file just leaves defaults in place.
type Config struct {
	DataDir         string
	ListenAddr      string
	XrayBin         string
	EntryIP         string
	EgressIP        string
	PanelDomain     string
	PollIntervalSec int
}

// Load reads the env file (path from FLYPANEL_ENV, default ./.env) and
// returns a Config. A missing file is not an error.
func Load() (*Config, error) {
	cfg := &Config{
		DataDir:         "./data",
		ListenAddr:      "127.0.0.1:3002",
		XrayBin:         "./bin/xray",
		PollIntervalSec: 30,
	}
	path := os.Getenv("FLYPANEL_ENV")
	if path == "" {
		path = "./.env"
	}
	if err := cfg.applyFile(path); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) applyFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read env file: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if err := c.set(strings.TrimSpace(key), strings.TrimSpace(value)); err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) set(key, value string) error {
	switch key {
	case "DATA_DIR":
		c.DataDir = value
	case "LISTEN_ADDR":
		c.ListenAddr = value
	case "XRAY_BIN":
		c.XrayBin = value
	case "ENTRY_IP":
		c.EntryIP = value
	case "EGRESS_IP":
		c.EgressIP = value
	case "PANEL_DOMAIN":
		c.PanelDomain = value
	case "POLL_INTERVAL_SEC":
		if value == "" {
			return nil
		}
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("POLL_INTERVAL_SEC=%q: %w", value, err)
		}
		c.PollIntervalSec = n
	}
	return nil
}
