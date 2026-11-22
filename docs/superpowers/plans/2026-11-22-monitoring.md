# Monitoring: logs and Caddy metrics — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a «Мониторинг» page with two views — logs (server journald, xray file, Caddy) and Caddy metric charts backed by a 15-second scrape of Caddy's admin `/metrics`.

**Architecture:** New Go package `internal/monitoring` reads logs (journalctl JSON via an injectable runner, plus a plain file tail for xray) and parses Prometheus text. Samples land in SQLite (`caddy_samples`, 30-day retention) from a goroutine in `serve`; two API routes serve entries and aggregated series; one Svelte page renders both views with the existing uPlot.

**Tech Stack:** Go 1.2x stdlib (`os/exec`, `encoding/json`, `net/http`), SQLite via existing `store`, Svelte 5 runes, uPlot, vitest.

**Spec:** `docs/superpowers/specs/2026-11-22-monitoring-design.md`

## Global Constraints

- Panel runs as `flypanel`, `ProtectSystem=strict`, `ReadWritePaths=/opt/flypanel`; it must never write outside `DATA_DIR` and never edit `/etc/caddy`.
- All existing Russian UI strings are translated through `t()` from `$lib/i18n.svelte`; new keys go into FLAT dotted-key files `web/src/lib/locales/{ru,en,de}.ts`, inserted immediately before the closing `} as const;` / `};`. `en`/`de` are typed `Record<keyof typeof ru, string>` — every new key needs all three.
- Backend error codes are snake_case via `jsonError`; frontend maps known codes through `errText` (`web/src/lib/errors.ts`).
- Log/monitoring endpoints are admin-only and denied to `read`-scope API tokens.
- Default log length 200, max 1000. Metric scrape interval 15 s, retention 30 days.

## Review Focus

1. A `journalctl` failure (no `systemd-journal` group, no binary on dev machines) must surface as 502 `journal_unavailable`, never as a blank page or a hang.
2. A missing `DATA_DIR/xray-error.log` is an empty list (200), not an error — xray has never logged yet.
3. Counter resets and the first point of a range must not produce negative or absurd rates.
4. Log messages from journald arrive as either a JSON string or an array of byte values; both must render.
5. An empty metrics table must show «Данных пока нет», and missing `caddy_reverse_proxy_upstreams_healthy` must render «—», not a red/green guess.

---

### Task 1: Log reader package

**Files:**
- Create: `internal/monitoring/logs.go`
- Test: `internal/monitoring/logs_test.go`

**Interfaces:**
- Produces:
  - `type Entry struct { Time time.Time; Source string; Level string; Message string }` (JSON tags `time`, `source`, `level`, `message`)
  - `type Runner func(name string, args ...string) ([]byte, error)`
  - `type Reader struct { DataDir string; Run Runner }`
  - `func (r Reader) Read(source string, lines int) ([]Entry, error)` — `source` is `server`, `caddy` or `xray`; unknown source returns error `monitoring: invalid source`.
  - `const maxLines = 1000`, `const defaultLines = 200`

- [ ] **Step 1: Write the failing test**

```go
func TestReadJournalJSON(t *testing.T) {
	var gotArgs []string
	r := Reader{Run: func(name string, args ...string) ([]byte, error) {
		gotArgs = append([]string{name}, args...)
		return []byte(`{"__REALTIME_TIMESTAMP":"1763800000000000","PRIORITY":"3","MESSAGE":"boom","SYSLOG_IDENTIFIER":"caddy","_SYSTEMD_UNIT":"caddy.service"}
{"__REALTIME_TIMESTAMP":"1763800001000000","PRIORITY":"6","MESSAGE":[104,105],"SYSLOG_IDENTIFIER":"caddy"}
not json
`), nil
	}}
	got, err := r.Read("caddy", 50)
	// want 2 entries: level "err" message "boom", level "info" message "hi"; bad line skipped
	// want gotArgs to contain "-u" "caddy" "-n" "50" and "--no-pager"
}

func TestReadXrayFile(t *testing.T) {
	// temp file with 3 lines: "[Warning] a", "[Error] b", "plain"
	// Read("xray", 2) → 2 entries, levels "warning","error" (last two lines), source "xray"
}

func TestReadXrayMissingFile(t *testing.T) {
	// Read("xray", 10) on empty dir → empty slice, nil error
}

func TestReadClampsLines(t *testing.T) {
	// lines 0 → "-n" "200"; lines 5000 → "-n" "1000"
}

func TestReadInvalidSource(t *testing.T) { /* error, not panic */ }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/monitoring/ -run TestRead -v`
Expected: FAIL — `undefined: Reader` / package has no non-test files.

- [ ] **Step 3: Implement `internal/monitoring/logs.go`**

journald path: `Run("journalctl", "-o", "json", "-n", strconv.Itoa(n), "--no-pager")` and, for `caddy`, insert `"-u", "caddy"` before `-o`. Parse each line into a struct with `__REALTIME_TIMESTAMP` (µs string), `PRIORITY`, `SYSLOG_IDENTIFIER`, and `MESSAGE` typed `json.RawMessage` decoded either as string or `[]byte`. Priority map 0..7 → `emerg|alert|crit|err|warning|notice|info|debug`; unknown → `info`. `Source` = requested source; for journald keep `SYSLOG_IDENTIFIER` in the entry only if needed — the response `source` field is the requested one.

xray path: read `<DataDir>/xray-error.log`, `os.IsNotExist` → empty slice; split by `\n`, drop trailing empty, take the last `n`; level from regex `\[(Warning|Error|Info)\]` (case-insensitive), default `info`; `Time` left zero when the line has no timestamp — the frontend renders an empty time cell.

Any `Run` error wraps as `fmt.Errorf("journalctl: %w", err)` so the handler can map it to 502.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/monitoring/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/monitoring/
git commit -m "feat(monitoring): log reader for journald and xray file"
```

---

### Task 2: xray error log file

**Files:**
- Modify: `internal/xray/config.go:19` (`GenerateConfig` signature), `internal/xray/config.go` log object, `internal/inbounds/apply.go:59`
- Test: `internal/xray/config_test.go` (update all calls), `internal/inbounds/apply.go` trim

**Interfaces:**
- Consumes: nothing from Task 1.
- Produces: `func GenerateConfig(inbounds []store.Inbound, activeUsers []store.User, routingJSON, egressIP string, apiPort int, logPath string) ([]byte, error)` — the generated `log` object becomes `{"loglevel":"warning","error":"<logPath>"}` when `logPath != ""`, otherwise unchanged.

- [ ] **Step 1: Write the failing test**

Add to `internal/xray/config_test.go`:

```go
func TestConfigXrayErrorLogPath(t *testing.T) {
	cfg, err := GenerateConfig(testInbounds(), nil, "", "", testAPIPort, "/opt/flypanel/data/xray-error.log")
	// decode, assert log.error == the path and log.loglevel == "warning"
}
```

- [ ] **Step 2: Run it**

Run: `go test ./internal/xray/ -run TestConfigXrayErrorLogPath`
Expected: FAIL — too many arguments / field empty.

- [ ] **Step 3: Implement**

Add the sixth parameter, set `"error": logPath` when non-empty. Update every existing call in `config_test.go` with `""` and the single production call in `internal/inbounds/apply.go` to `a.Cfg.DataDir + "/xray-error.log"`.

- [ ] **Step 4: Trim oversized file at apply time**

In `internal/inbounds/apply.go` `Apply`, before regenerating config: if the file is larger than 5 MB, truncate it (`os.Truncate(path, 0)`), ignoring `os.IsNotExist`. One line of comment: `shortcut: ротация не нужна, xray пишет только warning+`.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/xray/ ./internal/inbounds/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/xray internal/inbounds
git commit -m "feat(xray): write errors to data dir log file"
```

---

### Task 3: Caddy samples storage

**Files:**
- Create: `internal/store/migrations/0007_caddy_samples.sql`
- Create: `internal/store/caddy_samples.go`
- Test: `internal/store/caddy_samples_test.go`
- Modify: `internal/store/store_test.go` (`allTables` gains `caddy_samples`, both expected schema versions 6 → 7)

**Interfaces:**
- Produces:
  - `type CaddySample struct { TS time.Time; Requests, Errors, InFlight, DurationSum, DurationCount, RSS float64; UpstreamHealthy *bool }`
  - `func (s *Store) CaddySampleInsert(CaddySample) error`
  - `func (s *Store) CaddySamplesSince(since time.Time) ([]CaddySample, error)` — ordered by `ts`
  - `func (s *Store) CaddySamplesPrune(before time.Time) error`

- [ ] **Step 1: Write the failing test**

```go
func TestCaddySampleLifecycle(t *testing.T) {
	// insert two samples (one with nil UpstreamHealthy, one true), CaddySamplesSince(base) → 2 rows in ts order,
	// nil stays nil and true stays true; prune with a cutoff between them leaves 1.
}
```

- [ ] **Step 2: Run it**

Run: `go test ./internal/store/ -run TestCaddySampleLifecycle`
Expected: FAIL — undefined methods.

- [ ] **Step 3: Implement migration and methods**

SQL from the spec: `upstream_healthy INTEGER` nullable, `CREATE INDEX idx_caddy_samples_ts ON caddy_samples(ts)`. Store `''`/`NULL` via a `*bool` → `any` helper (`nil`, `0`, `1`); scan back through `sql.NullInt64`. `TS` uses `fmtTime`. Prune uses `DELETE FROM caddy_samples WHERE ts < ?`.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/store/`
Expected: PASS, including `TestOpenMigrates` at version 7.

- [ ] **Step 5: Commit**

```bash
git add internal/store
git commit -m "feat(store): caddy metric samples table"
```

---

### Task 4: Caddy metrics scraper

**Files:**
- Create: `internal/monitoring/caddy.go`
- Test: `internal/monitoring/caddy_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks (documents its own parse).
- Produces:
  - `func Scrape(ctx context.Context, url string) (Sample, error)` — GET `<url>/metrics`, 5 s timeout, non-200 is an error.
  - `func parseMetrics(text string) Sample`
  - `type Sample struct { Requests, Errors, InFlight, DurationSum, DurationCount, RSS float64; UpstreamHealthy *bool }`

- [ ] **Step 1: Write the failing test**

```go
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
	// Requests 123, Errors 2, InFlight 1, DurationSum 6.5, DurationCount 123, RSS 55000000, UpstreamHealthy true
}

func TestParseMetricsNoUpstream(t *testing.T) { /* nil pointer */ }
func TestParseMetricsSkipsGarbage(t *testing.T) { /* lines without value do not panic */ }
```

- [ ] **Step 2: Run it**

Run: `go test ./internal/monitoring/ -run TestParse`
Expected: FAIL.

- [ ] **Step 3: Implement**

Iterate lines; skip `#`; split last space into name-labels / value; `strconv.ParseFloat` (scientific notation allowed), skip on error. Match by prefix before `{` or end of line: sum `caddy_http_requests_total`, `caddy_http_request_errors_total`, `caddy_http_requests_in_flight`, `..._duration_seconds_sum`, `..._duration_seconds_count`; take the value of `process_resident_memory_bytes`; `UpstreamHealthy` = min over `caddy_reverse_proxy_upstreams_healthy` values < 1 → false, else true; stays `nil` when no such line was seen.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/monitoring/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/monitoring
git commit -m "feat(monitoring): scrape caddy prometheus metrics"
```

---

### Task 5: API routes, aggregation and token deny

**Files:**
- Create: `internal/api/monitoring.go`
- Create: `internal/api/monitoring_test.go`
- Modify: `internal/api/router.go` (two routes), `internal/api/handlers.go` (`Handlers` gains `Monitor *monitoring.Reader`, `CaddyURL string`), `internal/auth/middleware.go` (`readDenied`)

**Interfaces:**
- Consumes: `monitoring.Reader.Read`, `store.CaddySamplesSince`, `monitoring.Sample`.
- Produces:
  - `GET /api/monitoring/logs?source=&lines=` → `{"entries":[Entry...]}`; 400 `invalid_source`; 502 `journal_unavailable` on reader error.
  - `GET /api/monitoring/caddy?range=` → `{"points":[{ts,rps,errors_per_s,avg_ms,inflight,rss,upstream_healthy}],"summary":{requests,errors,avg_ms,inflight,rss,upstream_healthy}}`; unknown range → 400 `invalid_range`.

- [ ] **Step 1: Write the failing test**

```go
func TestMonitoringLogs(t *testing.T) {
	// router with fake runner injected through Deps (see Interfaces): source=server → 200 entries; source=bogus → 400 invalid_source;
	// runner error → 502 journal_unavailable.
}

func TestMonitoringCaddySeries(t *testing.T) {
	// seed three samples 60 s apart with cumulative requests 100,160,220 → rps 1.0; avg_ms from duration sums;
	// one sample resets requests to 10 → that interval counts 0, not negative.
}

func TestMonitoringDeniedForReadToken(t *testing.T) {
	// read token GET /api/monitoring/logs → 403 (same as /api/config).
}
```

- [ ] **Step 2: Run it**

Run: `go test ./internal/api/ -run TestMonitoring`
Expected: FAIL — routes 404.

- [ ] **Step 3: Implement handlers**

`Deps` gains `Monitor *monitoring.Reader` (built in `main` with `DataDir` and the real runner; tests set `Run`). Aggregation: pick bucket per range (`1h`→15s, `6h`/`24h`→60s, `7d`→15m), fetch `CaddySamplesSince(from.Add(-bucket))`, compute per-bucket deltas of `Requests`/`Errors`/`DurationSum`/`DurationCount`, take last gauge values per bucket; `requests`/`errors` in summary = sum of positive deltas, `avg_ms` = `1000*Δsum/Δcount`, gauges from the last sample. Negative deltas are clamped to 0 (Caddy restarted).

- [ ] **Step 4: Wire and deny**

Router: `p.HandleFunc("GET /api/monitoring/logs", apiH.MonitoringLogs)` and `.../caddy`. Middleware `readDenied`: add `strings.HasPrefix(path, "/api/monitoring")` to the deny branch.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/api/ ./internal/auth/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/api internal/auth
git commit -m "feat(api): monitoring logs and caddy series endpoints"
```

---

### Task 6: Scrape goroutine and config field

**Files:**
- Modify: `internal/config/config.go` (field + `CADDY_ADMIN_URL`), `cmd/flypanel/main.go` (goroutine), `internal/monitoring/caddy.go` (runner loop)

**Interfaces:**
- Produces: `func RunScraper(ctx context.Context, st *store.Store, url string, every time.Duration, logf func(string, ...any))` — ticker loop, insert sample, hourly prune, `logf` only on state change (ok→fail, fail→ok).

- [ ] **Step 1: Write the failing test**

```go
func TestRunScraperLogsOncePerFailure(t *testing.T) {
	// httptest server returning 500; run RunScraper with every=10ms for ~50ms with ctx cancel;
	// count log calls: first failure logs once, repeats do not.
}
```

- [ ] **Step 2: Run it** — `go test ./internal/monitoring/ -run TestRunScraper` → FAIL.

- [ ] **Step 3: Implement**

Config: add `CaddyAdminURL string` default `http://127.0.0.1:2019`, case `"CADDY_ADMIN_URL"`. `main.go`: `go monitoring.RunScraper(ctx, st, cfg.CaddyAdminURL, 15*time.Second, log.Printf)`.

- [ ] **Step 4: Run tests** — `go test ./...` → PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config cmd internal/monitoring
git commit -m "feat(monitoring): scrape caddy every 15s"
```

---

### Task 7: Monitoring page — logs view

**Files:**
- Create: `web/src/routes/Monitoring.svelte`
- Create: `web/src/routes/Monitoring.test.ts`
- Modify: `web/src/App.svelte` (route `/monitoring` → Monitoring, nav item `nav.monitoring`), locale files (`nav.monitoring` + `monitor.*` + `errors.journal_unavailable`)

**Interfaces:**
- Consumes: `GET /api/monitoring/logs`.
- Produces: routes `/monitoring` and `/monitoring/caddy`, view switch buttons («Логи» / «Caddy»).

- [ ] **Step 1: Write the failing test**

```ts
// «логи грузятся по источнику»: stub fetch /api/monitoring/logs → {entries:[{time,source:'caddy',level:'err',message:'boom'}]};
// expects text 'boom'; click 'xray' → last request URL contains source=xray
// «английский язык переводит страницу мониторинга»: setLang('en') → 'Logs', 'Caddy', no 'Логи'
```

- [ ] **Step 2: Run it** — `cd web && npx vitest run src/routes/Monitoring.test.ts` → FAIL.

- [ ] **Step 3: Implement page**

State: `view: 'logs' | 'caddy'`, `source = 'server'`, `entries`, `lines = 200`, `auto = false`, `err`. `loadLogs()` calls `/api/monitoring/logs?source=${source}&lines=${lines}`, maps `journal_unavailable` via `errText`. Auto-refresh with `$effect` + `setInterval` cleaned on destroy. Markup: page title, two switch buttons, source buttons (server/xray/caddy), refresh, checkbox «Автообновление (5 с)», monospace list; level colors emerald/amber/red via `level === 'err'` etc.

- [ ] **Step 4: Run tests and checks**

Run: `cd web && npx vitest run && npm run check`
Expected: all green, 0 errors.

- [ ] **Step 5: Commit**

```bash
git add web/src
git commit -m "feat(web): monitoring page with log viewer"
```

---

### Task 8: Monitoring page — Caddy charts

**Files:**
- Modify: `web/src/routes/Monitoring.svelte`, `web/src/routes/Monitoring.test.ts`, locale files (`monitor.*` keys)

**Interfaces:**
- Consumes: `GET /api/monitoring/caddy?range=`.
- Produces: charts inside the same page.

- [ ] **Step 1: Write the failing test**

```ts
// «caddy показывает точки»: stub → points [{ts, rps:1, errors_per_s:0, avg_ms:12, inflight:0, rss:1e8, upstream_healthy:true}];
// switch to Caddy view, wait for summary card text '1' requests, assert uPlot constructed (mock like TrafficChart.test.ts)
// «пустые метрики не падают»: points: [] → text 'Нет данных'
```

- [ ] **Step 2: Run it** → FAIL.

- [ ] **Step 3: Implement**

Range buttons 1ч/6ч/24ч/7д, summary cards (requests, errors, avg ms, in-flight, RSS MB, upstream «ok»/«—»/«down»), two charts mirroring `TrafficChart.svelte` (`new UPlot(...)` on a `bind:this` div, destroy on unmount, rebuild on `i18n.lang` and on data change).

- [ ] **Step 4: Run tests and checks** — `cd web && npx vitest run && npm run check`.

- [ ] **Step 5: Commit**

```bash
git add web/src
git commit -m "feat(web): caddy metric charts"
```

---

### Task 9: Deploy documentation

**Files:**
- Modify: `docs/deploy-checklist.md`

- [ ] **Step 1: Add the ops steps** — `SupplementaryGroups=systemd-journal` in the unit; Caddyfile global `{ metrics }` + site `log` with backup/reload/rollback; verification `curl -s 127.0.0.1:2019/metrics | grep caddy_http_requests_total`; note that `CADDY_ADMIN_URL` defaults to `http://127.0.0.1:2019`.

- [ ] **Step 2: Commit**

```bash
git add docs/deploy-checklist.md
git commit -m "docs: monitoring deploy steps"
```