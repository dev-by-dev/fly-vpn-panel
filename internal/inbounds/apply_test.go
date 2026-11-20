package inbounds

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"flypanel/internal/config"
	"flypanel/internal/store"
	"flypanel/internal/xray"
)

var errFakeValidate = errors.New("fake validate failure")

func okValidate(string, []byte) error { return nil }

func failValidate(string, []byte) error { return errFakeValidate }

// testEnv wires the Applier and Handlers against a real store and a fake
// xray binary that records every run invocation (each Manager Start).
type testEnv struct {
	st      *store.Store
	srv     http.Handler
	ap      *Applier
	mgr     *xray.Manager
	cfgPath string
	counter string
}

func newTestEnv(t *testing.T, validate func(string, []byte) error) *testEnv {
	t.Helper()
	dir := t.TempDir()
	counter := filepath.Join(dir, "xray-starts")
	bin := fakeXray(t, counter)
	cfgPath := filepath.Join(dir, "xray.json")
	mgr := xray.NewManager(bin, cfgPath)
	t.Cleanup(func() { _ = mgr.Stop() })
	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ap := NewApplier(st, &config.Config{
		DataDir:  dir,
		XrayBin:  bin,
		EgressIP: "203.0.113.1", // TEST-NET-3; no real infrastructure
	}, mgr)
	ap.Validate = validate
	h := &Handlers{Store: st, Applier: ap}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/inbounds", h.List)
	mux.HandleFunc("POST /api/inbounds", h.Create)
	mux.HandleFunc("GET /api/inbounds/{id}", h.Get)
	mux.HandleFunc("PATCH /api/inbounds/{id}", h.Patch)
	mux.HandleFunc("DELETE /api/inbounds/{id}", h.Delete)
	mux.HandleFunc("POST /api/inbounds/{id}/keys", h.RegenKeys)
	mux.HandleFunc("GET /api/config", h.GetConfig)
	mux.HandleFunc("GET /api/config/routing", h.GetRouting)
	mux.HandleFunc("PUT /api/config/routing", h.PutRouting)
	return &testEnv{st: st, srv: mux, ap: ap, mgr: mgr, cfgPath: cfgPath, counter: counter}
}

// fakeXray writes a shell-script xray binary that answers the version
// probe, appends one line per `run` invocation to counter, then sleeps.
func fakeXray(t *testing.T, counter string) string {
	t.Helper()
	// shortcut: macOS sleep lacks "infinity", so 24h stands in for it.
	bin := filepath.Join(t.TempDir(), "xray")
	script := `#!/bin/sh
if [ "$1" = "version" ]; then
	echo "Xray 25.11.20 (flypanel test fake)"
	exit 0
fi
echo run >> "` + counter + `"
exec sleep 86400
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake xray: %v", err)
	}
	return bin
}

// startCount counts recorded xray run invocations (one per Start/Restart).
func startCount(t *testing.T, counter string) int {
	t.Helper()
	b, err := os.ReadFile(counter)
	if errors.Is(err, fs.ErrNotExist) {
		return 0
	}
	if err != nil {
		t.Fatalf("read start counter: %v", err)
	}
	return strings.Count(string(b), "\n")
}

// waitStarts polls until the fake xray has recorded want invocations; the
// child echoes asynchronously after cmd.Start returns.
func waitStarts(t *testing.T, counter string, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if startCount(t, counter) >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("xray starts never reached %d within %v", want, timeout)
}

func seedInbound(t *testing.T, st *store.Store, tag string, enabled bool) {
	t.Helper()
	err := st.InboundCreate(&store.Inbound{
		Tag:     tag,
		Enabled: enabled,
		ConfigJSON: `{"tag":"` + tag + `","port":443,` +
			`"protocol":"vless","settings":{}}`,
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("seed inbound %q: %v", tag, err)
	}
}

func TestApplyInvalidKeepsOld(t *testing.T) {
	env := newTestEnv(t, failValidate)
	seedInbound(t, env.st, "in-a", true)
	if err := os.WriteFile(env.cfgPath, []byte("old config"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := env.ap.Apply(`{"domainStrategy":"AsIs","rules":[]}`)
	if err == nil {
		t.Fatal("Apply() = nil, want validate error")
	}

	got, err := os.ReadFile(env.cfgPath)
	if err != nil || string(got) != "old config" {
		t.Fatalf("old file after failed apply = %q (err %v), want %q", got, err, "old config")
	}
	if n := startCount(t, env.counter); n != 0 {
		t.Fatalf("xray starts = %d, want 0 (no restart after validate failure)", n)
	}
	if env.mgr.Status().Running {
		t.Fatal("manager running after validate failure, want never started")
	}
}

func TestApplyValidRestarts(t *testing.T) {
	env := newTestEnv(t, okValidate)
	seedInbound(t, env.st, "in-a", true)
	if err := os.WriteFile(env.cfgPath, []byte("old config"), 0o644); err != nil {
		t.Fatal(err)
	}

	routing := `{"domainStrategy":"IPIfNonEqualMatch",` +
		`"rules":[{"type":"field","domain":["example.com"],"outboundTag":"direct"}]}`
	if err := env.ap.Apply(routing); err != nil {
		t.Fatalf("Apply(): %v", err)
	}
	waitStarts(t, env.counter, 1, 2*time.Second)

	got, err := os.ReadFile(env.cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) == "old config" || !json.Valid(got) {
		t.Fatalf("config file not updated: %s", got)
	}
	for _, want := range []string{"IPIfNonEqualMatch", "in-a", "203.0.113.1", `"port": 443`} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("applied config missing %q: %s", want, got)
		}
	}
	if !env.mgr.Status().Running {
		t.Fatal("manager not running after apply")
	}
	// Exactly one restart: let any stray echo settle, then the count must
	// not have grown past 1.
	time.Sleep(200 * time.Millisecond)
	if n := startCount(t, env.counter); n != 1 {
		t.Fatalf("xray starts = %d, want exactly 1", n)
	}
}

func TestApplyAtomicFailureKeepsOld(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	// A read-only config dir makes the temp write fail before any rename,
	// so the existing file must survive byte-for-byte.
	cfgDir := filepath.Join(dir, "cfg")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(cfgDir, "xray.json")
	if err := os.WriteFile(cfgPath, []byte("old config"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(cfgDir, 0o755) })
	if err := os.Chmod(cfgDir, 0o555); err != nil {
		t.Fatal(err)
	}

	counter := filepath.Join(dir, "xray-starts")
	bin := fakeXray(t, counter)
	mgr := xray.NewManager(bin, cfgPath)
	t.Cleanup(func() { _ = mgr.Stop() })
	ap := NewApplier(st, &config.Config{
		DataDir:  cfgDir,
		XrayBin:  bin,
		EgressIP: "203.0.113.1", // TEST-NET-3
	}, mgr)
	ap.Validate = okValidate

	if err := ap.Apply(""); err == nil {
		t.Fatal("Apply() = nil, want atomic-write error")
	}
	got, err := os.ReadFile(cfgPath)
	if err != nil || string(got) != "old config" {
		t.Fatalf("old file after failed write = %q (err %v), want %q", got, err, "old config")
	}
	if n := startCount(t, counter); n != 0 {
		t.Fatalf("xray starts = %d, want 0 (no restart after write failure)", n)
	}
}
