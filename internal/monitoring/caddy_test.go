package monitoring

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const metricsFixture = `# HELP caddy_http_requests_total Counter of HTTP requests.
# TYPE caddy_http_requests_total counter
caddy_http_requests_total{code="200",handler="reverse_proxy",server="srv0"} 120
caddy_http_requests_total{code="404",handler="reverse_proxy",server="srv0"} 3
caddy_http_request_errors_total{handler="reverse_proxy"} 2
caddy_http_requests_in_flight{server="srv0"} 1
caddy_http_request_duration_seconds_sum{code="200"} 6.5
caddy_http_request_duration_seconds_count{code="200"} 123
process_resident_memory_bytes 5.5e+07
caddy_reverse_proxy_upstreams_healthy{upstream="127.0.0.1:3002"} 1
`

func TestParseMetrics(t *testing.T) {
	got := parseMetrics(metricsFixture)
	if got.Requests != 123 {
		t.Errorf("Requests = %v, want 123", got.Requests)
	}
	if got.Errors != 2 {
		t.Errorf("Errors = %v, want 2", got.Errors)
	}
	if got.InFlight != 1 {
		t.Errorf("InFlight = %v, want 1", got.InFlight)
	}
	if got.DurationSum != 6.5 || got.DurationCount != 123 {
		t.Errorf("duration = %v/%v, want 6.5/123", got.DurationSum, got.DurationCount)
	}
	if got.RSS != 5.5e7 {
		t.Errorf("RSS = %v, want 5.5e7", got.RSS)
	}
	if got.UpstreamHealthy == nil || !*got.UpstreamHealthy {
		t.Errorf("UpstreamHealthy = %v, want true", got.UpstreamHealthy)
	}
}

func TestParseMetricsNoUpstream(t *testing.T) {
	got := parseMetrics("caddy_http_requests_total 5\n")
	if got.UpstreamHealthy != nil {
		t.Errorf("UpstreamHealthy = %v, want nil", *got.UpstreamHealthy)
	}
}

func TestParseMetricsSkipsGarbage(t *testing.T) {
	got := parseMetrics("garbage line without value\nnan_value abc\ncaddy_http_requests_total 7\n")
	if got.Requests != 7 {
		t.Errorf("Requests = %v, want 7", got.Requests)
	}
}

func TestScrape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics" {
			t.Errorf("path = %s, want /metrics", r.URL.Path)
		}
		_, _ = w.Write([]byte(metricsFixture))
	}))
	defer srv.Close()
	got, err := Scrape(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Scrape: %v", err)
	}
	if got.Requests != 123 {
		t.Errorf("Requests = %v", got.Requests)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer bad.Close()
	if _, err := Scrape(context.Background(), bad.URL); err == nil {
		t.Fatal("Scrape(500) = nil error, want error")
	}
}

func TestRunScraperLogsOncePerFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	var logs []string
	RunScraper(ctx, nil, srv.URL, 10*time.Millisecond, func(format string, args ...any) {
		logs = append(logs, format)
	})
	if len(logs) != 1 {
		t.Fatalf("log calls = %d, want 1 (state change only): %v", len(logs), logs)
	}
}
