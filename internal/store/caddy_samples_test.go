package store

import (
	"testing"
	"time"
)

func boolp(v bool) *bool { return &v }

func TestCaddySampleLifecycle(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	defer s.Close()
	base := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	first := CaddySample{TS: base, Requests: 100, Errors: 1, InFlight: 2, DurationSum: 3.5, DurationCount: 10, RSS: 5e7}
	second := CaddySample{TS: base.Add(time.Minute), Requests: 160, Errors: 2, InFlight: 1, DurationSum: 7, DurationCount: 20, RSS: 6e7, UpstreamHealthy: boolp(true)}
	for _, in := range []CaddySample{first, second} {
		if err := s.CaddySampleInsert(in); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	got, err := s.CaddySamplesSince(base)
	if err != nil {
		t.Fatalf("since: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("samples = %d, want 2", len(got))
	}
	if got[0].Requests != 100 || got[1].Requests != 160 {
		t.Errorf("order/values = %v", got)
	}
	if got[0].UpstreamHealthy != nil {
		t.Errorf("first upstream = %v, want nil (metric absent)", *got[0].UpstreamHealthy)
	}
	if got[1].UpstreamHealthy == nil || !*got[1].UpstreamHealthy {
		t.Errorf("second upstream = %v, want true", got[1].UpstreamHealthy)
	}
	if err := s.CaddySamplesPrune(base.Add(30 * time.Second)); err != nil {
		t.Fatalf("prune: %v", err)
	}
	got, _ = s.CaddySamplesSince(base)
	if len(got) != 1 || got[0].Requests != 160 {
		t.Fatalf("after prune = %v, want only the second row", got)
	}
}
