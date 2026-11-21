package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"flypanel/internal/api"
	"flypanel/internal/config"
	"flypanel/internal/store"
	"flypanel/internal/xray"
)

// fakeServeXray writes a shell script standing in for the xray binary:
// `version` answers Manager's startup probe, `run -test` stands in for
// config validation, and `run -c <path>` parks only when the config file
// exists — exiting non-zero otherwise, like real xray with a missing
// config. Only this spawned script is ever signalled by the test.
func fakeServeXray(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "xray")
	body := `if [ "$1" = "version" ]; then
	echo "Xray fake (flypanel serve test)"
	exit 0
fi
if [ "$2" = "-test" ]; then
	exit 0
fi
if [ -f "$3" ]; then
	exec sleep 86400
fi
echo "fake xray: config missing: $3" >&2
exit 1
`
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatalf("write fake xray: %v", err)
	}
	return path
}

// TestApplyInitialXrayConfig proves the serve startup bug: seeding the
// store alone never writes data/xray.json, so starting xray directly
// crash-loops on a missing config. The initial apply must write the
// config file and leave xray running on it.
func TestApplyInitialXrayConfig(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("store.Open(): %v", err)
	}
	defer st.Close()
	if err := api.Seed(st); err != nil {
		t.Fatalf("api.Seed(): %v", err)
	}

	cfg := &config.Config{DataDir: dir, XrayBin: fakeServeXray(t), ListenAddr: "127.0.0.1:0"}
	mgr := xray.NewManager(cfg.XrayBin, filepath.Join(dir, "xray.json"))
	defer mgr.Stop()

	if err := applyInitialXrayConfig(st, cfg, mgr); err != nil {
		t.Fatalf("applyInitialXrayConfig(): %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "xray.json")); err != nil {
		t.Fatalf("xray.json not written: %v", err)
	}
	// A missing config would make the fake exit instantly; a survivor
	// past this delay proves it parked on a real config file.
	time.Sleep(300 * time.Millisecond)
	if s := mgr.Status(); !s.Running || s.PID <= 0 {
		t.Fatalf("mgr.Status() = %+v, want running with a PID", s)
	}
}
