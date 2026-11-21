package xray

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Status is a point-in-time snapshot of the xray child process.
type Status struct {
	Running  bool
	PID      int
	Uptime   time.Duration
	Restarts int
	Version  string
}

// Manager owns the xray child process: start, graceful stop (SIGTERM with a
// 5s deadline, then SIGKILL), and automatic restart after a crash with
// exponential backoff (1s doubling, capped at 30s).
type Manager struct {
	binPath string
	cfgPath string

	mu        sync.Mutex
	cmd       *exec.Cmd
	running   bool
	startedAt time.Time
	restarts  int
	version   string
	stopped   bool
	stopCh    chan struct{}
	wg        sync.WaitGroup
}

// NewManager returns a Manager for the given xray binary and config path.
// Nothing is started until Start is called.
func NewManager(binPath, cfgPath string) *Manager {
	return &Manager{binPath: binPath, cfgPath: cfgPath}
}

// versionRe вытаскивает номер из первой строки `xray version`, вида
// «Xray 26.11.9 (Xray, Penetrates Everything.) …»: остальное — шум для UI.
var versionRe = regexp.MustCompile(`\d+\.\d+\.\d+`)

func parseVersion(out string) string {
	line, _, _ := strings.Cut(out, "\n")
	if v := versionRe.FindString(line); v != "" {
		return v
	}
	return strings.TrimSpace(line)
}

// Start launches xray, captures its version, and begins supervising it.
// It is a no-op if xray is already running or a crash restart is pending.
func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running || (m.stopCh != nil && !m.stopped) {
		return nil
	}
	m.version = ""
	if out, err := exec.Command(m.binPath, "version").Output(); err == nil {
		m.version = parseVersion(string(out))
	}
	cmd := exec.Command(m.binPath, "run", "-c", m.cfgPath)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("xray start: %w", err)
	}
	m.stopCh = make(chan struct{})
	m.stopped = false
	m.cmd = cmd
	m.running = true
	m.startedAt = time.Now()
	m.wg.Add(1)
	go m.supervise(cmd)
	return nil
}

// Stop terminates xray gracefully: SIGTERM, up to 5s wait, then SIGKILL.
// It blocks until the child is fully reaped, so no zombie is left behind.
func (m *Manager) Stop() error {
	m.mu.Lock()
	if m.cmd == nil {
		m.mu.Unlock()
		return nil
	}
	if !m.stopped {
		m.stopped = true
		close(m.stopCh)
	}
	cmd := m.cmd
	m.mu.Unlock()

	if cmd.Process != nil {
		_ = cmd.Process.Signal(syscall.SIGTERM)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && m.Status().Running {
		time.Sleep(50 * time.Millisecond)
	}
	m.mu.Lock()
	if m.running && m.cmd != nil && m.cmd.Process != nil {
		_ = m.cmd.Process.Kill()
	}
	m.running = false
	m.cmd = nil
	m.mu.Unlock()

	m.wg.Wait()
	return nil
}

// Restart stops xray (fully) and starts it again.
func (m *Manager) Restart() error {
	if err := m.Stop(); err != nil {
		return err
	}
	return m.Start()
}

// Status returns the current snapshot of the child process.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := Status{Running: m.running, Restarts: m.restarts, Version: m.version}
	if m.running && m.cmd != nil && m.cmd.Process != nil {
		st.PID = m.cmd.Process.Pid
		st.Uptime = time.Since(m.startedAt)
	}
	return st
}

// supervise reaps the child and restarts it after crashes, growing the
// backoff from 1s to a 30s cap. It exits when Stop closes stopCh.
func (m *Manager) supervise(cmd *exec.Cmd) {
	defer m.wg.Done()
	for {
		_ = cmd.Wait()
		m.mu.Lock()
		m.running = false
		if m.stopped {
			m.cmd = nil
			m.mu.Unlock()
			return
		}
		m.restarts++
		stopCh := m.stopCh
		backoff := backoffFor(m.restarts)
		m.mu.Unlock()

		select {
		case <-stopCh:
			return
		case <-time.After(backoff):
		}

		m.mu.Lock()
		if m.stopped {
			m.mu.Unlock()
			return
		}
		next := exec.Command(m.binPath, "run", "-c", m.cfgPath)
		if err := next.Start(); err != nil {
			m.mu.Unlock()
			continue // retry on the next loop iteration, after another backoff
		}
		m.cmd = next
		m.running = true
		m.startedAt = time.Now()
		cmd = next
		m.mu.Unlock()
	}
}

// backoffFor returns the sleep before restart number `restarts`:
// 1s, 2s, 4s, 8s, 16s, then capped at 30s.
func backoffFor(restarts int) time.Duration {
	d := time.Second
	for range max(restarts-1, 0) {
		d *= 2
		if d >= 30*time.Second {
			return 30 * time.Second
		}
	}
	return d
}
