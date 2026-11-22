package monitoring

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"flypanel/internal/store"
)

// Sample is one Caddy metrics scrape. Counters are cumulative.
type Sample struct {
	Requests        float64
	Errors          float64
	InFlight        float64
	DurationSum     float64
	DurationCount   float64
	RSS             float64
	UpstreamHealthy *bool
}

// Scrape fetches and parses Caddy's admin /metrics endpoint.
func Scrape(ctx context.Context, url string) (Sample, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(url, "/")+"/metrics", nil)
	if err != nil {
		return Sample{}, fmt.Errorf("caddy metrics: %w", err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return Sample{}, fmt.Errorf("caddy metrics: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Sample{}, fmt.Errorf("caddy metrics: status %d", res.StatusCode)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return Sample{}, fmt.Errorf("caddy metrics: %w", err)
	}
	return parseMetrics(string(body)), nil
}

// parseMetrics reads the Prometheus text format Caddy exposes. Unknown
// metrics are ignored; malformed lines are skipped.
func parseMetrics(text string) Sample {
	var s Sample
	var upstreams []float64
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		field := strings.LastIndexByte(line, ' ')
		if field < 0 {
			continue
		}
		name := line[:field]
		if i := strings.IndexByte(name, '{'); i >= 0 {
			name = name[:i]
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(line[field+1:]), 64)
		if err != nil {
			continue
		}
		switch name {
		case "caddy_http_requests_total":
			s.Requests += v
		case "caddy_http_request_errors_total":
			s.Errors += v
		case "caddy_http_requests_in_flight":
			s.InFlight += v
		case "caddy_http_request_duration_seconds_sum":
			s.DurationSum += v
		case "caddy_http_request_duration_seconds_count":
			s.DurationCount += v
		case "process_resident_memory_bytes":
			s.RSS = v
		case "caddy_reverse_proxy_upstreams_healthy":
			upstreams = append(upstreams, v)
		}
	}
	if len(upstreams) > 0 {
		sort.Float64s(upstreams)
		ok := upstreams[0] >= 1
		s.UpstreamHealthy = &ok
	}
	return s
}

// RunScraper samples Caddy every `every` until ctx is done, storing rows
// and pruning older than 30 days. logf fires only when the reachability
// state flips, so a stopped Caddy does not fill the journal.
func RunScraper(ctx context.Context, st *store.Store, url string, every time.Duration, logf func(string, ...any)) {
	if every <= 0 {
		every = 15 * time.Second
	}
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	failing := false
	lastPrune := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		s, err := Scrape(ctx, url)
		if err != nil {
			if !failing {
				logf("monitoring: caddy metrics unavailable: %v", err)
				failing = true
			}
			continue
		}
		if failing {
			logf("monitoring: caddy metrics recovered")
			failing = false
		}
		if st == nil {
			continue
		}
		sample := store.CaddySample{
			TS: time.Now().UTC(), Requests: s.Requests, Errors: s.Errors,
			InFlight: s.InFlight, DurationSum: s.DurationSum,
			DurationCount: s.DurationCount, RSS: s.RSS,
			UpstreamHealthy: s.UpstreamHealthy,
		}
		if err := st.CaddySampleInsert(sample); err != nil {
			logf("monitoring: store caddy sample: %v", err)
			continue
		}
		if time.Since(lastPrune) > time.Hour {
			if err := st.CaddySamplesPrune(time.Now().Add(-30 * 24 * time.Hour)); err != nil {
				logf("monitoring: prune caddy samples: %v", err)
			}
			lastPrune = time.Now()
		}
	}
}
