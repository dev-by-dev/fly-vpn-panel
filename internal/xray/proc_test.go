package xray

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

const fakeVersionLine = "Xray 25.11.20 (flypanel test fake)"

// fakeXrayVersion handles the `version` probe Manager runs at startup.
const fakeXrayVersion = `if [ "$1" = "version" ]; then
	echo "` + fakeVersionLine + `"
	exit 0
fi
`

// fakeXray writes a shell script standing in for the xray binary and returns
// its path. Only this spawned script is ever signalled by the tests.
func fakeXray(t *testing.T, body string) string {
	t.Helper()
	// shortcut: macOS sleep lacks "infinity", so 24h stands in for it.
	path := filepath.Join(t.TempDir(), "xray")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+fakeXrayVersion+body), 0o755); err != nil {
		t.Fatalf("write fake xray: %v", err)
	}
	return path
}

// waitRestarts polls until Status().Restarts reaches want.
func waitRestarts(t *testing.T, m *Manager, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if m.Status().Restarts >= want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("Restarts never reached %d within %v, last status %+v", want, timeout, m.Status())
}

// processGone fails if pid still exists — signal 0 succeeds on zombies too,
// so passing here means the child was fully reaped, not left undead.
func processGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if syscall.Kill(pid, 0) != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("pid %d still present after Stop (zombie or leaked?)", pid)
}

// assertNoLeftovers scans the process table for anything still referencing
// the fake binary path (dead or alive).
func assertNoLeftovers(t *testing.T, binPath string) {
	t.Helper()
	out, err := exec.Command("ps", "-axo", "stat=,command=").Output()
	if err != nil {
		t.Fatalf("ps: %v", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, binPath) {
			t.Errorf("process left behind after Stop: %s", strings.TrimSpace(line))
		}
	}
}

func TestManagerStartStop(t *testing.T) {
	bin := fakeXray(t, "exec sleep 86400\n")
	m := NewManager(bin, filepath.Join(filepath.Dir(bin), "config.json"))

	if err := m.Start(); err != nil {
		t.Fatalf("Start(): %v", err)
	}
	st := m.Status()
	if !st.Running || st.PID <= 0 {
		t.Fatalf("Status() after Start = %+v, want running with a PID", st)
	}
	if st.Version != "25.11.20" {
		t.Errorf("Version = %q, want %q", st.Version, "25.11.20")
	}
	if st.Uptime < 0 {
		t.Errorf("Uptime = %v, want >= 0", st.Uptime)
	}
	pid := st.PID

	if err := m.Stop(); err != nil {
		t.Fatalf("Stop(): %v", err)
	}
	if m.Status().Running {
		t.Error("Status().Running after Stop = true, want false")
	}
	processGone(t, pid)
	assertNoLeftovers(t, bin)

	if err := m.Restart(); err != nil {
		t.Fatalf("Restart(): %v", err)
	}
	st2 := m.Status()
	if !st2.Running || st2.PID <= 0 {
		t.Fatalf("Status() after Restart = %+v, want running with a PID", st2)
	}
	if err := m.Stop(); err != nil {
		t.Fatalf("Stop() after Restart: %v", err)
	}
	processGone(t, st2.PID)
	assertNoLeftovers(t, bin)
}

func TestManagerConcurrentRestart(t *testing.T) {
	bin := fakeXray(t, "exec sleep 86400\n")
	m := NewManager(bin, filepath.Join(filepath.Dir(bin), "config.json"))

	if err := m.Start(); err != nil {
		t.Fatalf("Start(): %v", err)
	}

	const n = 8
	errCh := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%4 == 3 {
				errCh <- m.Start() // no-op, must not race
			} else {
				errCh <- m.Restart()
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent op: %v", err)
		}
	}

	st := m.Status()
	if !st.Running || st.PID <= 0 {
		t.Fatalf("Status() after concurrent restarts = %+v, want running with a PID", st)
	}
	pid := st.PID
	if err := m.Stop(); err != nil {
		t.Fatalf("Stop(): %v", err)
	}
	if m.Status().Running {
		t.Error("Status().Running after Stop = true, want false")
	}
	processGone(t, pid)
	assertNoLeftovers(t, bin)
}

func TestManagerCrashBackoff(t *testing.T) {
	bin := fakeXray(t, "exit 1\n")
	m := NewManager(bin, filepath.Join(filepath.Dir(bin), "config.json"))

	if err := m.Start(); err != nil {
		t.Fatalf("Start(): %v", err)
	}
	// The script dies immediately; the manager must count the crash and
	// respawn it. Two crashes also prove the backoff loop keeps going.
	waitRestarts(t, m, 2, 8*time.Second)

	if err := m.Stop(); err != nil {
		t.Fatalf("Stop(): %v", err)
	}
	if m.Status().Running {
		t.Error("Status().Running after Stop = true, want false")
	}
	if m.Status().Restarts < 2 {
		t.Errorf("Restarts after Stop = %d, want >= 2", m.Status().Restarts)
	}
	assertNoLeftovers(t, bin)
}

func TestApplyAtomicFailureKeepsOld(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "xray.json")
	if err := os.WriteFile(cfgPath, []byte("old config"), 0o644); err != nil {
		t.Fatalf("write old config: %v", err)
	}

	// Target directory does not exist: the temp write fails, rename never
	// happens, and the old file must be untouched.
	if err := ApplyAtomic(filepath.Join(dir, "nope", "xray.json"), []byte("new config")); err == nil {
		t.Error("ApplyAtomic(missing dir) = nil error, want error")
	}
	got, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read old config: %v", err)
	}
	if string(got) != "old config" {
		t.Errorf("old file content after failed apply = %q, want %q", got, "old config")
	}

	// Happy path: the rename swaps the content atomically.
	if err := ApplyAtomic(cfgPath, []byte("new config")); err != nil {
		t.Fatalf("ApplyAtomic(): %v", err)
	}
	got, err = os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read new config: %v", err)
	}
	if string(got) != "new config" {
		t.Errorf("file content after apply = %q, want %q", got, "new config")
	}
}

func TestParseVersion(t *testing.T) {
	cases := map[string]string{
		"Xray 26.11.9 (Xray, Penetrates Everything.) 676eaf6 (go1.27.3 linux/amd64)": "26.11.9",
		"Xray 25.11.20 (flypanel test fake)":                                         "25.11.20",
		"xray stub":                                                                  "xray stub",
	}
	for in, want := range cases {
		if got := parseVersion(in); got != want {
			t.Errorf("parseVersion(%q) = %q, want %q", in, got, want)
		}
	}
}
