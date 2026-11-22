package api

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"flypanel/internal/auth"
	"flypanel/internal/config"
	"flypanel/internal/monitoring"
	"flypanel/internal/stats"
	"flypanel/internal/store"
	"flypanel/internal/xray"
)

// newMonitoringRouter is newTestRouter plus an injected log reader.
func newMonitoringRouter(t *testing.T, run monitoring.Runner) (http.Handler, *store.Store, *auth.JWT) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open(): %v", err)
	}
	t.Cleanup(func() { st.Close() })
	j := auth.NewJWT([]byte("0123456789abcdef0123456789abcdef"))
	cfg := &config.Config{
		DataDir: t.TempDir(), EntryIP: "203.0.113.10",
		PanelDomain: "panel.example.com", PollIntervalSec: 30,
	}
	h := NewRouter(Deps{
		Store:   st,
		JWT:     j,
		RL:      auth.NewRateLimiter(5, time.Minute),
		Manager: &fakeManager{status: xray.Status{Running: true}},
		Cfg:     cfg,
		Poller:  stats.NewPoller(st, nil, nil, 30*time.Second),
		Monitor: &monitoring.Reader{DataDir: cfg.DataDir, Run: run},
	})
	return h, st, j
}

func TestMonitoringLogs(t *testing.T) {
	line := `{"__REALTIME_TIMESTAMP":"1763800000000000","PRIORITY":"3","MESSAGE":"boom"}`
	h, _, j := newMonitoringRouter(t, func(name string, args ...string) ([]byte, error) {
		return []byte(line), nil
	})
	c := accessCookie(t, j)

	rr := doReq(t, h, "GET", "/api/monitoring/logs?source=server", c, nil)
	if rr.Code != 200 {
		t.Fatalf("status = %d, body %s", rr.Code, rr.Body)
	}
	m := bodyJSON(t, rr)
	entries, _ := m["entries"].([]any)
	if len(entries) != 1 {
		t.Fatalf("entries = %v, want 1", m["entries"])
	}
	if got := entries[0].(map[string]any)["message"]; got != "boom" {
		t.Errorf("message = %v, want boom", got)
	}

	rr = doReq(t, h, "GET", "/api/monitoring/logs?source=bogus", c, nil)
	if rr.Code != 400 || bodyJSON(t, rr)["error"] != "invalid_source" {
		t.Errorf("bad source: status %d body %s", rr.Code, rr.Body)
	}
}

func TestMonitoringLogsJournalFailure(t *testing.T) {
	h, _, j := newMonitoringRouter(t, func(name string, args ...string) ([]byte, error) {
		return nil, errors.New("no journal files")
	})
	rr := doReq(t, h, "GET", "/api/monitoring/logs?source=server", accessCookie(t, j), nil)
	if rr.Code != 502 || bodyJSON(t, rr)["error"] != "journal_unavailable" {
		t.Errorf("status %d body %s", rr.Code, rr.Body)
	}
}

func TestMonitoringCaddySeries(t *testing.T) {
	h, st, j := newMonitoringRouter(t, nil)
	base := time.Now().UTC().Add(-3 * time.Minute).Truncate(15 * time.Second)
	yes := true
	rows := []store.CaddySample{
		{Requests: 100, DurationSum: 1, DurationCount: 10},
		{Requests: 160, DurationSum: 3, DurationCount: 20},
		{Requests: 220, DurationSum: 6, DurationCount: 30, UpstreamHealthy: &yes},
	}
	for i, r := range rows {
		r.TS = base.Add(time.Duration(i) * 15 * time.Second)
		if err := st.CaddySampleInsert(r); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	rr := doReq(t, h, "GET", "/api/monitoring/caddy?range=1h", accessCookie(t, j), nil)
	if rr.Code != 200 {
		t.Fatalf("status = %d, body %s", rr.Code, rr.Body)
	}
	m := bodyJSON(t, rr)
	summary, _ := m["summary"].(map[string]any)
	if summary["requests"] != float64(120) {
		t.Errorf("summary.requests = %v, want 120", summary["requests"])
	}
	if summary["avg_ms"] != float64(250) {
		t.Errorf("summary.avg_ms = %v, want 250", summary["avg_ms"])
	}
	if summary["upstream_healthy"] != true {
		t.Errorf("upstream_healthy = %v, want true", summary["upstream_healthy"])
	}
	points, _ := m["points"].([]any)
	if len(points) == 0 {
		t.Fatal("no points")
	}

	rr = doReq(t, h, "GET", "/api/monitoring/caddy?range=99h", accessCookie(t, j), nil)
	if rr.Code != 400 || bodyJSON(t, rr)["error"] != "invalid_range" {
		t.Errorf("bad range: status %d body %s", rr.Code, rr.Body)
	}
}

// A Caddy restart resets the counters: the reset interval must count as
// zero, never as a negative rate.
func TestMonitoringCaddyCounterReset(t *testing.T) {
	h, st, j := newMonitoringRouter(t, nil)
	base := time.Now().UTC().Add(-2 * time.Minute).Truncate(15 * time.Second)
	rows := []store.CaddySample{{Requests: 500}, {Requests: 10}, {Requests: 40}}
	for i, r := range rows {
		r.TS = base.Add(time.Duration(i) * 15 * time.Second)
		if err := st.CaddySampleInsert(r); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	m := bodyJSON(t, doReq(t, h, "GET", "/api/monitoring/caddy?range=1h", accessCookie(t, j), nil))
	summary, _ := m["summary"].(map[string]any)
	if summary["requests"] != float64(30) {
		t.Errorf("summary.requests = %v, want 30", summary["requests"])
	}
}
