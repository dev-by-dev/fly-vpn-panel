package xray

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Validate writes cfg to a temp file and runs `xray run -test -c <file>`.
// xray's stderr is embedded in the returned error.
func Validate(binPath string, cfg []byte) error {
	tmp, err := os.CreateTemp("", "flypanel-validate-*.json")
	if err != nil {
		return fmt.Errorf("validate: temp file: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(cfg); err != nil {
		tmp.Close()
		return fmt.Errorf("validate: write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("validate: close temp: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, binPath, "run", "-test", "-c", tmp.Name())
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("validate: timed out after 10s: %s", strings.TrimSpace(stderr.String()))
		}
		return fmt.Errorf("validate: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// ApplyAtomic writes cfg to path via a temp file in the same directory plus
// os.Rename, so readers never observe a partial config. A failed write
// leaves any existing file at path untouched.
func ApplyAtomic(path string, cfg []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".flypanel-xray-*.json")
	if err != nil {
		return fmt.Errorf("apply: temp file: %w", err)
	}
	name := tmp.Name()
	defer os.Remove(name) // no-op once the rename succeeded
	if _, err := tmp.Write(cfg); err != nil {
		tmp.Close()
		return fmt.Errorf("apply: write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("apply: close temp: %w", err)
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("apply: rename: %w", err)
	}
	return nil
}
