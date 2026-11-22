package api

import (
	"net/http"
	"sort"
	"strconv"
	"time"

	"flypanel/internal/store"
)

type caddyPoint struct {
	TS              string  `json:"ts"`
	RPS             float64 `json:"rps"`
	ErrorsPerS      float64 `json:"errors_per_s"`
	AvgMS           float64 `json:"avg_ms"`
	InFlight        float64 `json:"inflight"`
	RSS             float64 `json:"rss"`
	UpstreamHealthy *bool   `json:"upstream_healthy"`
}

type caddySummary struct {
	Requests        float64 `json:"requests"`
	Errors          float64 `json:"errors"`
	AvgMS           float64 `json:"avg_ms"`
	InFlight        float64 `json:"inflight"`
	RSS             float64 `json:"rss"`
	UpstreamHealthy *bool   `json:"upstream_healthy"`
}

// MonitoringLogs serves recent log entries of one source
// (server, caddy or xray). lines is clamped to 1..1000, default 200.
func (h *Handlers) MonitoringLogs(w http.ResponseWriter, r *http.Request) {
	source := r.URL.Query().Get("source")
	if source == "" {
		source = "server"
	}
	lines, err := strconv.Atoi(r.URL.Query().Get("lines"))
	if err != nil || lines <= 0 {
		lines = 200
	}
	if lines > 1000 {
		lines = 1000
	}
	if source != "server" && source != "caddy" && source != "xray" {
		jsonError(w, http.StatusBadRequest, "invalid_source")
		return
	}
	if h.Monitor == nil {
		jsonError(w, http.StatusBadGateway, "journal_unavailable")
		return
	}
	entries, err := h.Monitor.Read(source, lines)
	if err != nil {
		jsonError(w, http.StatusBadGateway, "journal_unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

var caddyRanges = map[string]struct {
	window time.Duration
	bucket time.Duration
}{
	"1h":  {time.Hour, 15 * time.Second},
	"6h":  {6 * time.Hour, time.Minute},
	"24h": {24 * time.Hour, time.Minute},
	"7d":  {7 * 24 * time.Hour, 15 * time.Minute},
}

type bucketAcc struct {
	ts              time.Time
	requests        float64
	errors          float64
	durationSum     float64
	durationCount   float64
	inFlight        float64
	rss             float64
	upstreamHealthy *bool
}

// MonitoringCaddy aggregates stored Caddy samples into ~360 points for
// the requested range. Rates come from counter deltas; reset counters
// are clamped to zero instead of producing negative rates.
func (h *Handlers) MonitoringCaddy(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("range")
	if name == "" {
		name = "1h"
	}
	rng, ok := caddyRanges[name]
	if !ok {
		jsonError(w, http.StatusBadRequest, "invalid_range")
		return
	}
	now := time.Now().UTC()
	from := now.Add(-rng.window)
	// One extra sample before the window gives the first point a delta.
	rows, err := h.Store.CaddySamplesSince(from.Add(-rng.bucket))
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}

	seconds := rng.bucket.Seconds()
	buckets := map[int64]*bucketAcc{}
	order := []int64{}
	var prev *store.CaddySample
	for i := range rows {
		s := rows[i]
		if prev != nil && !s.TS.Before(from) {
			key := s.TS.Truncate(rng.bucket).Unix()
			acc, ok := buckets[key]
			if !ok {
				acc = &bucketAcc{ts: s.TS.Truncate(rng.bucket)}
				buckets[key] = acc
				order = append(order, key)
			}
			acc.requests += clampDelta(s.Requests - prev.Requests)
			acc.errors += clampDelta(s.Errors - prev.Errors)
			acc.durationSum += clampDelta(s.DurationSum - prev.DurationSum)
			acc.durationCount += clampDelta(s.DurationCount - prev.DurationCount)
			acc.inFlight = s.InFlight
			acc.rss = s.RSS
			acc.upstreamHealthy = s.UpstreamHealthy
		}
		prev = &s
	}

	sort.Slice(order, func(a, b int) bool { return order[a] < order[b] })
	points := make([]caddyPoint, 0, len(order))
	summary := caddySummary{}
	var sumDuration, sumCount float64
	for _, key := range order {
		acc := buckets[key]
		p := caddyPoint{
			TS:              acc.ts.Format(time.RFC3339),
			RPS:             acc.requests / seconds,
			ErrorsPerS:      acc.errors / seconds,
			InFlight:        acc.inFlight,
			RSS:             acc.rss,
			UpstreamHealthy: acc.upstreamHealthy,
		}
		if acc.durationCount > 0 {
			p.AvgMS = 1000 * acc.durationSum / acc.durationCount
		}
		points = append(points, p)
		summary.Requests += acc.requests
		summary.Errors += acc.errors
		sumDuration += acc.durationSum
		sumCount += acc.durationCount
		if len(order) > 0 && key == order[len(order)-1] {
			summary.InFlight = acc.inFlight
			summary.RSS = acc.rss
			summary.UpstreamHealthy = acc.upstreamHealthy
		}
	}
	if sumCount > 0 {
		summary.AvgMS = 1000 * sumDuration / sumCount
	}

	writeJSON(w, http.StatusOK, map[string]any{"points": points, "summary": summary})
}

func clampDelta(v float64) float64 {
	if v < 0 {
		return 0
	}
	return v
}
