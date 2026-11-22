# fly-vpn-panel v1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A single Go binary `flypanel` (SPA inside) + child xray: a complete VPN control panel (VLESS+XHTTP+Reality) with users, quotas, subscriptions and visibility into "who is online + how much traffic".

**Architecture:** Go stdlib net/http + SQLite (WAL) = source of truth; xray v26.11.9 — child process, config generated from the DB; structural edits go through `xray run -test` → atomic write → child restart; users/quotas/expiry — hot via gRPC AlterInbound; a 30 s poller collects traffic and online IPs from the xray stats API and applies enforcement. The SPA (Svelte 5) is embedded via embed.FS. Deployment — on a dedicated VPS (ADR-0008) behind a subdomain of the operator's own native Caddy, VPN — Reality on its own port.

**Tech Stack:** Go ≥1.22 (stdlib mux), modernc.org/sqlite v1.62.0, github.com/xtls/xray-core v26.11.9 (gRPC types; the Go pin = pseudo-version `v1.260327.1-0.20261109052709-676eaf66f8f0` — module path without `/v26`, the literal tag is invalid for Go modules; commit = tag v26.11.9, verified with ls-remote), golang-jwt/jwt v5.3.1, pquerna/otp v1.5.0, golang.org/x/crypto (argon2), google/uuid v1.6.0; Svelte 5.59 + Vite 8.3.8 + @sveltejs/vite-plugin-svelte 7.3.1 + Tailwind 4.3.5 + shadcn-svelte 1.8.1, svelte-spa-router, qrcode (QR), uPlot (charts), vitest.

**Spec:** `docs/superpowers/specs/2026-11-20-fly-vpn-panel-design.md` (approved 2026-11-20). ADR: `docs/adr/` 0001–0008 accepted (0001 amended by 0008).

## Global Constraints

- The repo is **public**: no real IPs/domains/names/paths to secrets in any artifact (code, tests, fixtures, docs). Placeholders: `<ENTRY_IP>`, `<PANEL_DOMAIN>`, `<EGRESS_IP>` (optional; empty → sendThrough is omitted). Fixtures use TEST-NET addresses (203.0.113.x / 198.51.100.x). Real values live only in the server-side `/opt/flypanel/.env` (chmod 600, gitignored) + the operator's private runbook outside the repo. Grep check before every push.
- Branches `<feature>-20261120-NN` from `dev`, merged into `dev`. Each Task = its own branch (named in the heading).
- Go ≥1.22, stdlib `net/http` router (Go 1.22+ patterns). No framework needed.
- SQLite DSN: `_journal_mode=WAL&_busy_timeout=5000&_synchronous=FULL` (modernc v1.62.0 only recognises `_synchronous`/`_sync`; a bare `synchronous=` is silently ignored — verified against the driver sources, Task 1). A single writer. Daily backup, 7 copies.
- xray **v26.11.9** (pinned; Go module: pseudo-version `v1.260327.1-0.20261109052709-676eaf66f8f0` = the exact tag commit, module path without `/v26`). API inbound dokodemo-door `127.0.0.1:10811` tag `api`. Policy level 0: `statsUserUplink/statsUserDownlink/statsUserOnline = true`.
- VPN: VLESS+XHTTP+Reality `0.0.0.0:8443`; entry `<ENTRY_IP>`; freedom `sendThrough <EGRESS_IP>` only if EGRESS_IP is set (dedicated VPS: one IP, the field is omitted). Reality dest defaults to `www.microsoft.com:443`, path `/xhttp`, mode `stream-up`, fp `chrome`.
- Panel bind: `127.0.0.1:3002` (prod and dev; LISTEN_ADDR from .env). Dedicated VPS (ADR-0008): 1 vCPU / 1 GB / 10 GB, Ubuntu 24.04, one IP (in=out), create 2 GB swap at deploy, no Docker needed, native Caddy. Do not touch the provider agent (8080/8189).
- Auth: one admin, CLI `flypanel admin init`, NO public registration. argon2id. JWT access 15 min + refresh 7 days, httpOnly cookies SameSite=Strict, refresh in the DB with revocation. TOTP optional → mandatory once enabled; 10 backup codes (hashes, one-time); disabling only with a valid code. Login rate limit 5/min/IP.
- Users: quota_bytes (0=unlimited), quota_period none|day|week|month, **up+down combined**, the counter = aggregate of deltas since quota_reset_at; device_limit (default 0) by **concurrent distinct online IPs**, auto-restore; expiry auto-disable/enable; disabled_reason quota|device_limit|expired|manual; manual is never auto-enabled. Membership v1: all active users in all enabled inbounds.
- Subscriptions: `https://<PANEL_DOMAIN>/sub/<token>` + `/base64 /json /mihomo /clash`; UA fallback: v2rayNG→base64, FlClash / clash-verge (lowercase)→mihomo, anything else→base64. Templates stored raw in the DB, **no sanitizer anywhere**. Defaults mirror production (read-only reading is permitted to the operator; paths — private runbook).
- gRPC poller 30 s (configurable). xray counters reset on restart — loss ≤ the interval.
- UI: **Russian**, dark theme. SPA embed.FS. Node ≥20.19/22.12 at build time only. Pin npm versions at start (they may have drifted).
- Server actions: operator SSH tooling only, **explicit permission for every action**, never print secrets. The old server with the live stack is physically separate — DO NOT TOUCH (the only contact: read-only reading of prod templates, Task 8). Results go into the operator's private journal (outside the repo).
- Timestamps in the DB and API: RFC3339 **UTC**. Quota period boundaries — UTC (day 00:00, week Monday 00:00, month 1st 00:00).

## Review Focus

1. **Restarting xray mid-period** zeroes the counters → the delta must not go negative or double up; the cursor survives a restart of the panel itself. → Task 9 tests `TestDeltaReset`, `TestCursorSurvivesPanelRestart`.
2. **Period rollover for a user disabled by quota** → auto-enable exactly at the boundary (UTC month/week edges). → Task 6 `TestNextBoundary`, `TestDecideRolloverEnables`.
3. **Invalid raw config/routing** → the old config untouched, no partial write, no restart. → Task 7 `TestApplyAtomicFailureKeepsOld`, `TestInvalidRouting422`.
4. **Auth edges**: expired/forged JWT, backup code reuse, the 6th login attempt within a minute, login without TOTP when it is enabled. → Task 4, the corresponding tests.
5. **Subscriptions**: unknown token → 404 without leaking the user's existence; expired/disabled → 403 + hot disable; UA `ClashVerge/*` (CamelCase) → base64 (not a broken clash). → Task 8 `TestSubAccess`, `TestFormatForUA`.

---

### Task 1: Scaffold + config + SQLite core

**Branch:** `store-20261120-01`

**Files:**
- Create: `go.mod`, `Makefile`, `.env.example`, `cmd/flypanel/main.go`, `internal/config/config.go`, `internal/store/store.go`, `internal/store/migrations.go`, `internal/store/migrations/0001_init.sql`
- Test: `internal/config/config_test.go`, `internal/store/store_test.go`

**Interfaces:**
- Consumes: —
- Produces:
  - `config.Load() (*Config, error)` — reads `.env` (path from `FLYPANEL_ENV`, default `./.env`; a missing file is OK). Fields: `DataDir string` (default `./data`), `ListenAddr string` (default `127.0.0.1:3002`), `XrayBin string` (default `./bin/xray`), `EntryIP, EgressIP, PanelDomain string` (from .env, may be empty in dev), `PollIntervalSec int` (default 30).
  - `store.Open(dataDir string) (*Store, error)` — creates dataDir, opens `panel.db` with the DSN pragmas, applies migrations. `(*Store).DB() *sql.DB`.
  - Migrations: embed `migrations/*.sql`, table `schema_version(version INTEGER)`, applied in a transaction in order, idempotently.

Schema `0001_init.sql` (all v1 tables, timestamps TEXT RFC3339 UTC):
- `admin(id INTEGER PK CHECK(id=1), username TEXT UNIQUE NOT NULL, password_hash TEXT NOT NULL, totp_secret TEXT, totp_enabled INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL)`
- `backup_codes(id INTEGER PK, code_hash TEXT NOT NULL, used INTEGER NOT NULL DEFAULT 0)`
- `users(id INTEGER PK, username TEXT UNIQUE NOT NULL, uuid TEXT NOT NULL, sub_token TEXT UNIQUE NOT NULL, expiry TEXT NOT NULL, quota_bytes INTEGER NOT NULL DEFAULT 0, quota_period TEXT NOT NULL DEFAULT 'none' CHECK(quota_period IN ('none','day','week','month')), quota_reset_at TEXT NOT NULL, device_limit INTEGER NOT NULL DEFAULT 0, disabled INTEGER NOT NULL DEFAULT 0, disabled_reason TEXT, created_at TEXT NOT NULL)`
- `inbounds(id INTEGER PK, tag TEXT UNIQUE NOT NULL, enabled INTEGER NOT NULL DEFAULT 1, config_json TEXT NOT NULL, created_at TEXT NOT NULL)` — `config_json` = the full inbound object without clients (clients are injected at generation time); the port lives inside config_json.
- `settings(key TEXT PK, value TEXT NOT NULL)`
- `traffic_samples(id INTEGER PK, username TEXT NOT NULL, ts TEXT NOT NULL, up_delta INTEGER NOT NULL, down_delta INTEGER NOT NULL)` + index `(username, ts)`
- `devices(id INTEGER PK, username TEXT NOT NULL, ip TEXT NOT NULL, first_seen TEXT NOT NULL, last_seen TEXT NOT NULL, UNIQUE(username, ip))`
- `refresh_tokens(id INTEGER PK, token_hash TEXT UNIQUE NOT NULL, expires_at TEXT NOT NULL, revoked INTEGER NOT NULL DEFAULT 0)`

`.gitignore` already exists at the repo root (secrets/DB/binaries/logs/backups/node_modules/dist, with an exception `!web/dist/index.html` for the embed placeholder) — check it and extend if needed. `.env.example` — placeholders only, EGRESS_IP empty by default. `Makefile`: `build` (npm ci+build → go build), `test` (go test ./...), `web-test`, `cross` (GOOS=linux GOARCH=amd64). `main.go` for now: a `version` subcommand, a `serve` stub (the full serve — Task 10).

- [ ] **Step 1: Failing tests**

`config_test.go`: `TestLoadDefaults` (empty env → DataDir `./data`, ListenAddr `127.0.0.1:3002`, PollIntervalSec 30, EgressIP empty); `TestLoadEnvFile` (temporary .env with `LISTEN_ADDR=203.0.113.1:3002`, `EGRESS_IP=198.51.100.7` → the fields match).
`store_test.go`: `TestOpenMigrates` (Open into a temporary dir → `schema_version`=1, all 8 tables exist); `TestOpenIdempotent` (a second Open does not fail, the version is the same); `TestWALPragmas` (`PRAGMA journal_mode` → `wal`, `PRAGMA busy_timeout` → 5000, `PRAGMA synchronous` → 2).

- [ ] **Step 2: Run — FAIL** (`go test ./...`: the packages do not compile)
- [ ] **Step 3: Implement** config (KEY=VALUE parser, no dependency), store.Open + migrations runner. The exact DSN pragmas from Global Constraints.
- [ ] **Step 4: Run — PASS** (`go test ./...`)
- [ ] **Step 5: Commit**

```bash
git checkout -b store-20261120-01 dev
git add -A && git commit -m "feat: scaffold, config, sqlite core + schema v1"
```

---

### Task 2: Store queries (all entities)

**Branch:** `store-20261120-02`

**Files:**
- Create: `internal/store/admin.go`, `internal/store/users.go`, `internal/store/inbounds.go`, `internal/store/settings.go`, `internal/store/samples.go`, `internal/store/devices.go`, `internal/store/tokens.go`, `internal/store/types.go`
- Test: `internal/store/queries_test.go`

**Interfaces:**
- Consumes: `store.Store` / `DB()` (Task 1)
- Produces (methods of `*Store`; types in `types.go`: `Admin{ID, Username, PasswordHash, TOTPSecret string, TOTPEnabled bool, CreatedAt time.Time}`, `User{ID int64, Username, UUID, SubToken string, Expiry time.Time, QuotaBytes int64, QuotaPeriod string, QuotaResetAt time.Time, DeviceLimit int, Disabled bool, DisabledReason string, CreatedAt time.Time}`, `Inbound{ID int64, Tag string, Enabled bool, ConfigJSON string, CreatedAt time.Time}`, `Device{Username, IP string, FirstSeen, LastSeen time.Time}`, `Bucket{TS time.Time, Up, Down int64}`):
  - `AdminGet() (*Admin, error)` (ErrNotFound if absent); `AdminCreate(username, hash string) error` (error if an admin already exists); `AdminUpdatePassword(hash string) error`; `AdminSetTOTP(secret string, enabled bool) error`
  - `BackupCodesReplace(hashes []string) error`; `BackupCodeConsume(hash string) (bool, error)` — atomically used=1, false if already used/absent
  - `UserCreate(*User) error`; `UserGet(id int64)`; `UserGetByToken(token string)`; `UserGetByUsername(name string)`; `UserList() ([]User, error)`; `UserUpdate(*User) error`; `UserDelete(id int64) error`; `UserSetDisabled(id int64, disabled bool, reason string) error` (reason="" when enabling)
  - `InboundCreate/Get/List/Update/Delete` (same shape); `InboundListEnabled() ([]Inbound, error)`
  - `SettingGet(key) (string, error)` (ErrNotFound); `SettingSet(key, value string) error`; `SettingsAll() (map[string]string, error)`
  - `SampleInsert(username string, ts time.Time, up, down int64) error`; `SamplesSince(username string, since time.Time) (up, down int64, err error)` — SUM; `TrafficSeries(username string, from, to time.Time, bucket string) ([]Bucket, error)` — bucket `hour|day`, GROUP BY; `PruneSamples(before time.Time) error`
  - `DeviceUpsert(username, ip string, now time.Time) error` — INSERT … ON CONFLICT(username,ip) UPDATE last_seen; `DevicesOnline(since time.Time) ([]Device, error)`; `DevicesForUser(username string) ([]Device, error)`; `PruneDevices(before time.Time) error`
  - `RefreshTokenInsert(hash string, exp time.Time) error`; `RefreshTokenValid(hash string) (bool, error)` — not revoked and not expired; `RefreshTokenRevoke(hash string) error`; `RefreshTokenRevokeAll() error`
  - `var ErrNotFound = errors.New("not found")`

- [ ] **Step 1: Failing tests** — a roundtrip per entity + edges: `TestUserCRUD` (create→get→update→list→delete; duplicate username → error); `TestUserSetDisabled` (reason written/cleared); `TestBackupCodeOneTime` (consume true → false again); `TestAdminSingle` (a second AdminCreate → error); `TestSamplesSince` (3 samples, since cuts off, SUM up+down separately); `TestTrafficSeries` (hour/day buckets aggregate); `TestDeviceUpsert` (first_seen does not change, last_seen is updated); `TestRefreshTokenLifecycle` (valid → revoke → invalid; expired → invalid); `TestSettingsRoundtrip`.
- [ ] **Step 2: FAIL**
- [ ] **Step 3: Implement** — database/sql, no ORM. Time: `time.Time.UTC().Format(time.RFC3339)` / parse.
- [ ] **Step 4: PASS**
- [ ] **Step 5: Commit** `git checkout -b store-20261120-02 dev && git add -A && git commit -m "feat: store queries for all entities"`

---

### Task 3: xray config generator + lifecycle + Reality keys

**Branch:** `xray-lifecycle-20261120-03`

**Files:**
- Create: `internal/xray/config.go`, `internal/xray/validate.go`, `internal/xray/proc.go`, `internal/xray/keys.go`, `internal/xray/testdata/golden.json`
- Test: `internal/xray/config_test.go`, `internal/xray/keys_test.go`, `internal/xray/proc_test.go`

**Interfaces:**
- Consumes: `store.Inbound`, `store.User` (Task 2)
- Produces:
  - `GenerateConfig(inbounds []store.Inbound, activeUsers []store.User, routingJSON, egressIP string, apiPort int) ([]byte, error)` — the full config: `log{loglevel:"warning"}`; `api{tag:"api",services:["HandlerService","StatsService"]}`; `policy{levels:{"0":{statsUserUplink:true,statsUserDownlink:true,statsUserOnline:true}},system:{statsInboundUplink:true,statsInboundDownlink:true}}`; inbounds: dokodemo-door `127.0.0.1:apiPort` tag `api` (sockopt none) + each enabled inbound from `ConfigJSON` with `settings.clients = [{id: uuid, email: username, level: 0} …]`; outbounds: `freedom` (tag `direct`, `sendThrough: egressIP` if non-empty) + `blackhole` tag `block`; `routing` = the parsed routingJSON (default `{"domainStrategy":"AsIs","rules":[]}`). Empty routingJSON → the default. Invalid JSON → error.
  - `Validate(binPath string, cfg []byte) error` — temp file, `exec.CommandContext(binPath, "run", "-test", "-c", tmp)`, 10 s timeout, stderr into the error text.
  - `ApplyAtomic(path string, cfg []byte) error` — temp in the same dir + `os.Rename`.
  - `NewManager(binPath, cfgPath string) *Manager`; `(*Manager).Start() error` / `Stop() error` / `Restart() error`; `Status() Status` where `Status{Running bool, PID int, Uptime time.Duration, Restarts int, Version string}`. A process crash → auto-restart with backoff 1s→2s→4s… cap 30s, a Restarts counter. Stop is graceful (SIGTERM, 5 s, then Kill). Version — `xray version` at start (first line).
  - `NewKeyPair() (private, public string, err error)` — `crypto/ecdh.X25519().GenerateKey(rand.Reader)`, base64.RawURLEncoding (xray format, 43 characters).

- [ ] **Step 1: Failing tests**

`config_test.go`: `TestGenerateGolden` — 1 enabled inbound (fixture: port 8443, Reality dest `www.microsoft.com:443`, test pbk/sid, path `/xhttp`, mode stream-up) + 2 active users + routing with the rule `{"type":"field","domain":["geosite:category-ads-all"],"outboundTag":"block"}` + egressIP `198.51.100.7` → the JSON is semantically equal to `testdata/golden.json` (comparison via unmarshal+reflect, key order irrelevant). `TestGenerateInjectsOnlyActive` — a disabled user does not appear in clients. `TestGenerateNoSanitizer` — routing with domain/ip rules passes through verbatim. `TestGenerateEmptyRouting` → the default. `TestGenerateBadRouting` → error. `TestGenerateNoEgress` — egressIP `""` → freedom without the sendThrough field (the dedicated VPS prod default: one IP).
`keys_test.go`: `TestKeyPairFormat` (43 chars, RawURL base64, pub deterministically derived from priv via ecdh).
`proc_test.go`: `TestManagerStartStop` (a fake "xray" — a shell script `sleep infinity` in a temporary dir; Start → Running, Stop → not Running); `TestManagerCrashBackoff` (a script `exit 1` → Restarts grows, the process is not a zombie); `TestApplyAtomicFailureKeepsOld` (rename into a non-existent dir → error, the old file intact). Validate with the real binary — integration behind the `XRAY_BIN` env (skip if absent): a valid golden → nil, broken JSON → error.

- [ ] **Step 2: FAIL**
- [ ] **Step 3: Implement.** Manager: one mutex, cmd.Process, a wait goroutine with backoff. The generator — `map[string]any` + json.MarshalIndent.
- [ ] **Step 4: PASS**
- [ ] **Step 5: Commit** `git checkout -b xray-lifecycle-20261120-03 dev && git add -A && git commit -m "feat: xray config generator, lifecycle manager, reality keygen"`

---

### Task 4: Auth (argon2 + JWT + TOTP + handlers + admin CLI)

**Branch:** `auth-20261120-04`

**Files:**
- Create: `internal/auth/password.go`, `internal/auth/jwt.go`, `internal/auth/totp.go`, `internal/auth/ratelimit.go`, `internal/auth/middleware.go`, `internal/auth/handlers.go`, `cmd/flypanel/admin.go`
- Test: `internal/auth/auth_test.go`, `internal/auth/handlers_test.go`

**Interfaces:**
- Consumes: store (Task 2)
- Produces:
  - `HashPassword(pw string) (string, error)` / `VerifyPassword(hash, pw string) bool` — argon2id time=1 memory=64MiB threads=4 keyLen=32 saltLen=16, PHC string `$argon2id$v=19$m=65536,t=1,p=4$…`.
  - `NewJWT(key []byte) *JWT`; `(*JWT).IssueAccess() (string, error)` — claims `{sub:"admin", exp: now+15m}`, HS256; `(*JWT).VerifyAccess(token string) error`; `IssueRefresh() (raw, hash string, err error)` — 32 random bytes base64url, hash=sha256; refresh exp 7d stored in the DB (Task 2).
  - `EnrollTOTP(account string) (secret, otpauthURI string, err error)` — pquerna/otp totp.Generate, issuer `flypanel`; `ValidateTOTP(secret, code string) bool` — totp.Validate (skew 1); `GenerateBackupCodes() (codes, hashes []string, err error)` — 10 codes of 8 base32 characters, hash=sha256.
  - `NewRateLimiter(limit int, window time.Duration) *RateLimiter`; `Allow(ip string) bool` — in-memory sliding window.
  - `Middleware(j *JWT) func(http.Handler) http.Handler` — the access cookie `fp_access`; absent/invalid → 401 JSON `{"error":"unauthorized"}`.
  - `Handlers{Store *store.Store, JWT *JWT, RL *RateLimiter}` with the methods `Login/Refresh/Logout/Enroll2FA/Verify2FA/Disable2FA http.HandlerFunc`. Cookie names: `fp_access` (15m), `fp_refresh` (7d, path `/api/auth`), httpOnly SameSite=Strict.
  - CLI `flypanel admin init`: interactive username + password (≥12 characters, confirmation; `golang.org/x/term.ReadPassword`), error if an admin exists. Flag `--password-stdin` — read the password from stdin without confirmation (for automation/smoke).

Login behaviour: rate limit by IP (5/min) → 429; wrong password → 401 `{"error":"invalid_credentials"}`; 2FA enabled and the code missing/invalid → 401 `{"error":"2fa_required"}`; the code = TOTP **or** a backup code (consume); success → cookies + `{"ok":true}`. Enroll2FA: generates a secret+URI+10 codes, stores the secret (totp_enabled stays 0), returns the codes ONCE; Verify2FA `{code}` valid → totp_enabled=1 + BackupCodesReplace; Disable2FA `{code}` valid → totp_enabled=0, clear the secret. Refresh: a valid refresh cookie → a new access (we do not rotate the refresh in v1); Logout: revoke the refresh + clear the cookies. Password change (Settings, Task 10) → RefreshTokenRevokeAll.

- [ ] **Step 1: Failing tests**

`auth_test.go`: `TestArgon2Roundtrip` (+wrong password false); `TestJWTExpired` (a token with exp in the past → error), `TestJWTTampered` (broken signature → error); `TestTOTPRFC6238` (secret `GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ`, Unix time 59, SHA1/6 digits → code `287082` valid via ValidateTOTP with a fixed clock — pquerna totp.ValidateCustom); `TestBackupCodes` (10 items, 8 characters); `TestRateLimiter` (5 allow, the 6th deny, allow again after the window).
`handlers_test.go` (httptest + store in a temporary dir): `TestLoginFlow` (admin init via AdminCreate+HashPassword → login ok cookies; wrong password 401); `TestLogin2FARequired` (2FA enabled → no code 401 `2fa_required`, with a valid TOTP ok, a backup code ok and 401 the second time); `TestLoginRateLimit` (6 requests → 429); `TestRefreshLogout` (refresh yields access; logout → refresh invalid); `TestMiddleware` (no cookie 401, with a valid one 200, expired 401); `TestAdminInitSecondFails`.

- [ ] **Step 2: FAIL**
- [ ] **Step 3: Implement**
- [ ] **Step 4: PASS**
- [ ] **Step 5: Commit** `git checkout -b auth-20261120-04 dev && git add -A && git commit -m "feat: auth argon2id, jwt, totp, backup codes, rate limit, admin cli"`

---

### Task 5: xray gRPC client (stats + handler)

**Branch:** `xray-grpc-20261120-05`

**Files:**
- Create: `internal/xrayclient/client.go`, `internal/xrayclient/fake_test.go`
- Test: `internal/xrayclient/client_test.go`
- Modify: `go.mod` (+ `github.com/xtls/xray-core` pseudo-version `v1.260327.1-0.20261109052709-676eaf66f8f0` = tag v26.11.9, `google.golang.org/grpc`)

**Interfaces:**
- Consumes: the xray-core module (types `app/stats/command`, `app/proxyman/command`), the config with the api inbound (Task 3)
- Produces:
  - `New(addr string) (*Client, error)` — grpc.NewClient insecure (loopback).
  - `(*Client).QueryUserStats(email string) (up, down int64, err error)` — QueryStats patterns `user>>><email>>>traffic>>>uplink` / `…downlink` (3 arrows — verified against the v26.11.9 sources, T5), `Reset_: false`; a missing counter → 0.
  - `(*Client).OnlineUsers() ([]string, error)` — GetAllOnlineUsers.
  - `(*Client).OnlineIPs(email string) ([]string, error)` — GetStatsOnlineIpList.
  - `(*Client).AddUser(inboundTag, uuid, email string) error` / `RemoveUser(inboundTag, email string) error` — AlterInbound AddUserOperation/RemoveUserOperation (user level 0); an "already exists" error → nil (idempotency).
  - `(*Client).Close() error`.

- [ ] **Step 1: Verify the semantics (spec §6.7).** Read the xray-core v26.11.9 sources (`app/stats/command/command.go`, `app/proxyman/command/command.go`): confirm that GetStatsOnlineIpList returns IPs per user and GetAllOnlineUsers returns the online emails; record the conclusion in the commit message. If the methods do not give IPs — STOP, tell the operator (access-log fallback is a separate decision, not a silent one). **Verified (T5): PASSED** — GetStatsOnlineIpList returns per-user IPs (map `user>>>email>>>online`), GetAllOnlineUsers — online emails; source: module cache v26.11.9.
- [ ] **Step 2: Failing tests** — a bufconn fake server (`fake_test.go` implements StatsServiceServer + HandlerServiceServer): `TestQueryUserStats` (exact patterns, reset=false, up/down mapped; missing → 0); `TestOnlineUsers`; `TestOnlineIPs`; `TestAddRemoveUser` (AlterInbound receives the right tag/operation; already-exists → nil).
- [ ] **Step 3: FAIL**
- [ ] **Step 4: Implement**
- [ ] **Step 5: PASS**
- [ ] **Step 6: Commit** `git checkout -b xray-grpc-20261120-05 dev && git add -A && git commit -m "feat: xray grpc client (stats, online ips, alter inbound)"`

---

### Task 6: Enforcement engine + users API

**Branch:** `users-20261120-06`

**Files:**
- Create: `internal/users/enforce.go`, `internal/users/sync.go`, `internal/users/handlers.go`
- Test: `internal/users/enforce_test.go`, `internal/users/handlers_test.go`

**Interfaces:**
- Consumes: store (Task 2), xrayclient (Task 5), google/uuid
- Produces:
  - `Action{Type string /* disable|enable|rollover */, Reason string}`.
  - `Decide(u store.User, usedBytes int64, onlineIPs []string, now time.Time) []Action` — a pure function. Rules (in order): (1) `expiry <= now` and !disabled → disable(expired); (2) period != none and `now >= NextBoundary(u.QuotaResetAt, period)` → rollover (**on rollover the used value for the quota check = 0** — the new period has just started); (3) quota_bytes>0 and used>=quota_bytes and !disabled → disable(quota); (4) disabled by quota and (after rollover used<quota_bytes) → enable; (5) device_limit>0 and `len(unique(onlineIPs)) > device_limit` and !disabled → disable(device_limit); (6) disabled by device_limit and len<=limit → enable; (7) disabled by expired and expiry>now → enable. **manual — never auto-enabled.** Several reasons at once: disable wins over enable; reason priority expiry > quota > device_limit.
  - `NextBoundary(start time.Time, period string) time.Time` — UTC: day → the next 00:00; week → the next Monday 00:00; month → the next 1st 00:00; none → start (unchanged).
  - `type MemberSync interface { AddUser(inboundTag, uuid, email string) error; RemoveUser(inboundTag, email string) error }` (satisfied by `*xrayclient.Client`).
  - `SyncMembership(ctx context.Context, c MemberSync, inbounds []store.Inbound, active []store.User) error` — reconcile: for each enabled inbound add all active users (idempotently), remove those not in active (by email). An error on one inbound does not interrupt the others (collected with errors.Join).
  - `Handlers{Store, Client, Manager}` — `/api/users` GET/POST, `/api/users/{id}` GET/PATCH/DELETE, `/api/users/{id}/reset-quota` POST. GET list enriches each user: `used_bytes` (SamplesSince(quota_reset_at), up+down) and `online_count` (DevicesForUser with last_seen ≥ now-2*interval) — for the UI table. POST: uuid v4, sub_token 16 characters base62 crypto/rand, quota_reset_at = the start of the current period (for day — today 00:00 UTC), expiry mandatory. PATCH: the username field is **immutable** (the stats key); quota_bytes/quota_period/expiry/device_limit/disabled(manual) are editable; changing quota_period → quota_reset_at = the start of the current period; a manual limit raise/reset → if disabled_reason=quota → enable. DELETE → RemoveUser from all enabled inbounds + UserDelete. Any change of disabled/expiry → an immediate hot sync (don't wait for the poller).

- [ ] **Step 1: Failing tests**

`enforce_test.go` — table-driven `TestDecide` (all transitions): quota exceeded→disable(quota); rollover+used<limit→enable; device over→disable; device back→enable; expiry passed→disable(expired); extended→enable; manual disabled + all conditions ok → NO enable; expiry+quota at once → disable(expired) takes priority; NAT: 5 connections from one IP = 1 distinct (onlineIPs are already distinct from the API — the test that Decide counts len, no duplicates). `TestNextBoundary`: vectors — day `2026-11-20T15:00Z`→`2026-11-21T00:00Z`; week Thursday→Monday; month `2026-11-20`→`2026-12-01T00:00Z`; `2026-12-31`→`2027-01-01T00:00Z`; none→unchanged. `TestSyncIdempotent` (fake MemberSync: a repeated add is not an error, removing an absent user is not an error).
`handlers_test.go`: `TestUserCreateDefaults` (uuid format, token 16, device_limit 0, quota_period none); `TestUserListEnriched` (used_bytes = the sum of samples since reset_at, online_count from fresh devices); `TestUserPatchQuotaRaiseEnables`; `TestUserDeleteRemovesFromInbounds`; `TestUsernameImmutable` (PATCH username → 400).

- [ ] **Step 2: FAIL**
- [ ] **Step 3: Implement**
- [ ] **Step 4: PASS**
- [ ] **Step 5: Commit** `git checkout -b users-20261120-06 dev && git add -A && git commit -m "feat: quota/device/expiry enforcement + users api"`

---

### Task 7: Inbounds API + config apply pipeline + routing

**Branch:** `inbounds-20261120-07`

**Files:**
- Create: `internal/inbounds/handlers.go`, `internal/inbounds/apply.go`
- Test: `internal/inbounds/apply_test.go`, `internal/inbounds/handlers_test.go`

**Interfaces:**
- Consumes: store, xray.GenerateConfig/Validate/ApplyAtomic/Manager/NewKeyPair (Task 3), xrayclient (Task 5), config.Config
- Produces:
  - `type Applier struct { Store *store.Store; Cfg *config.Config; Mgr *xray.Manager; Validate func(binPath string, cfg []byte) error }` — `NewApplier(...)` sets `Validate = xray.Validate`; tests inject a fake. The method `Apply(routingJSON string) error` — the pipeline: GenerateConfig(enabled inbounds, active users, routing, Cfg.EgressIP, 10811) → Validate(Cfg.XrayBin, …) → ApplyAtomic(Cfg.DataDir+"/xray.json") → Mgr.Restart. A Validate error → return the error, the file is NOT touched, there is NO restart.
  - Handlers: `/api/inbounds` GET/POST, `/api/inbounds/{id}` GET/PATCH/DELETE, `/api/inbounds/{id}/keys` POST (NewKeyPair → update privateKey/publicKey in config_json → Apply). POST/PATCH/DELETE inbound → Apply. Form validation: port 1–65535, tag unique, Reality dest non-empty.
  - `/api/config` GET → the current xray.json file as is; `/api/config/routing` GET → the settings key `routing` (default `{"domainStrategy":"AsIs","rules":[]}`); PUT → a JSON-parse check (syntax only, **no sanitizer whatsoever**) → SettingSet → Apply. Apply failure → 422 `{"error":…}`, the old routing value preserved (SettingSet rollback).
  - The default inbound on first install (seed in Task 10): tag `vless-xhttp`, port 8443, path `/xhttp`, mode stream-up, dest `www.microsoft.com:443`, serverNames `[dest-host]`, fp chrome, shortIds `[""]`→generate 8-hex at seed, keys via NewKeyPair.

- [ ] **Step 1: Failing tests**

`apply_test.go`: `TestApplyInvalidKeepsOld` (the fake Validate returns an error → the old file byte-for-byte, Manager.Restart not called — a spy); `TestApplyValidRestarts` (valid → the file updated, restart called once); `TestApplyAtomicFailureKeepsOld`.
`handlers_test.go`: `TestRoutingNoSanitizer` (PUT routing with domain+ip+geoip rules → GET returns VERBATIM the same JSON); `TestInvalidRouting422` (broken JSON → 422, the old value intact); `TestInboundCRUD` (create→list→patch port→delete; duplicate tag → 400); `TestKeysRegen` (POST keys → a new pair 43 chars, different from the old one).

- [ ] **Step 2: FAIL**
- [ ] **Step 3: Implement**
- [ ] **Step 4: PASS**
- [ ] **Step 5: Commit** `git checkout -b inbounds-20261120-07 dev && git add -A && git commit -m "feat: inbounds api, config apply pipeline, routing editor backend"`

---

### Task 8: Subscription generators + templates + /sub handlers

**Branch:** `subs-20261120-08`

**Files:**
- Create: `internal/sub/links.go`, `internal/sub/json.go`, `internal/sub/yaml.go`, `internal/sub/ua.go`, `internal/sub/handlers.go`, `internal/sub/defaults/` (templates), `internal/sub/defaults.go` (embed)
- Test: `internal/sub/sub_test.go`

**Interfaces:**
- Consumes: store, config.Config (EntryIP, PanelDomain)
- Produces:
  - `RenderBase64(inbounds []store.Inbound, u store.User, entryIP string) string` — one line per enabled inbound: `vless://<uuid>@<entryIP>:<port>?type=xhttp&path=<path>&mode=<mode>&security=reality&pbk=<pub>&sid=<first shortId>&sni=<first serverName>&fp=<fp>&encryption=none#<tag>` (query url-encoded), the result is base64.StdEncoding of the joined `\n`.
  - `RenderJSON(inbounds, u, entryIP, templateJSON string) ([]byte, error)` — templateJSON = a client config with the placeholders `{{UUID}} {{ADDRESS}} {{PORT}} {{PBK}} {{SID}} {{SNI}} {{FP}} {{PATH}} {{MODE}} {{TAG}}`; substitution per enabled inbound → an **array** of configs. A placeholder left after substitution → error.
  - `RenderYAML(template string, params map[string]string) (string, error)` — the same `{{KEY}}` substitution for mihomo/clash (text, we don't parse yaml — raw).
  - `FormatForUA(ua string) string` — lowered contains `v2rayng` → base64; lowered contains `flclash` → mihomo; **raw** contains `clash-verge` → mihomo; otherwise base64. (ClashVerge CamelCase → base64.)
  - Handlers `GET /sub/{token}` (UA fallback) and `GET /sub/{token}/{format}` (base64|json|mihomo|clash). Unknown token → 404 `{"error":"not found"}` (uniformly). An expired user → UserSetDisabled(expired) + 403 `{"error":"expired"}`; disabled → 403 `{"error":"disabled"}`. Content-Type: base64 text/plain, json application/json, yaml text/yaml. The header `Subscription-Userinfo: upload=…; download=…; total=…; expire=…` (bytes from samples, total=quota_bytes, expire=unix expiry) — client compatibility.
  - Templates: settings keys `template_json`, `template_mihomo`, `template_clash` (base64 — a structural link generator, no template). `/api/templates/{format}` GET/PUT (raw, no sanitizer; PUT only checks that the placeholders are not broken by a render test on a dummy user). Seed the defaults on first install (Task 10).
  - **The default templates mirror production** (spec §6.6): a gated step below.

- [ ] **Step 1: GATED — read the production templates (read-only).** Performed by the operator / the main session through SSH tooling, **an explicit "yes" for every action**, paths from the private runbook (not in the repo). Read the current mihomo/clash/xray-json templates of the running panel. Check the anonymisation (grep for real IPs/domains/names — there must be none in the templates, only the generator panel's placeholders). Adapt the placeholders to our `{{…}}` format, save into `internal/sub/defaults/`. **Diff verification**: the RU-direct rules, adblock (anti-ad.net + 10 Google/YouTube domains), sniffer, `ipv6: false`, Telegram CIDR → proxy no-resolve — present verbatim. If production is unreachable — STOP, ask the operator (don't invent defaults silently).
- [ ] **Step 2: Failing tests**

`TestRenderBase64` — decode, check EVERY query parameter of the link (type=xhttp, security=reality, pbk, sid, sni, fp, encryption=none, path, mode), host=entryIP, fragment=tag; 2 enabled inbounds → 2 lines.
`TestRenderJSONArray` — 2 inbounds → an array of len 2, uuid/address/port substituted, the routing from the template preserved verbatim (anti-sanitizer).
`TestRenderYAML` — substitution of all `{{…}}`, a leftover `{{` → error; rules/sniffer/ipv6 from the default in place.
`TestFormatForUA` — table: `v2rayNG/1.9.30`→base64, `FlClash/0.9.3 clash-verge`→mihomo, `clash-verge/2.5.10`→mihomo, `ClashVerge/2.2.3`→base64, `clash.meta`→base64, `Mozilla/5.0`→base64, empty→base64.
`TestSubAccess` (httptest): unknown token → 404 (the body identical for a non-existent and a deleted user); expired → 403 + the user disabled(expired) in the DB; disabled → 403; active + the /json suffix → a valid array; the base URL with a FlClash UA → yaml.
`TestTemplatePutNoSanitizer` — PUT a mihomo template with DOMAIN-SUFFIX/IP-CIDR rules → GET verbatim.

- [ ] **Step 3: FAIL**
- [ ] **Step 4: Implement**
- [ ] **Step 5: PASS**
- [ ] **Step 6: Commit** `git checkout -b subs-20261120-08 dev && git add -A && git commit -m "feat: subscription generators (4 formats), templates, sub handlers"`

---

### Task 9: Stats poller + enforcement wiring + aggregates + backup

**Branch:** `stats-20261120-09`

**Files:**
- Create: `internal/stats/poller.go`, `internal/stats/backup.go`
- Test: `internal/stats/poller_test.go`, `internal/stats/backup_test.go`

**Interfaces:**
- Consumes: store, xrayclient, users.Decide/NextBoundary/SyncMembership, xray.Manager
- Produces:
  - `type StatsSource interface { QueryUserStats(email string) (up, down int64, err error); OnlineUsers() ([]string, error); OnlineIPs(email string) ([]string, error) }` and `type MemberSync interface { AddUser(tag, uuid, email string) error; RemoveUser(tag, email string) error }` (both satisfied by `*xrayclient.Client`; fakes in tests).
  - `NewPoller(st *store.Store, src StatsSource, sync MemberSync, interval time.Duration) *Poller`; `(*Poller).SetInterval(d time.Duration)` — change the interval on the fly (from a settings PUT); `Run(ctx)` — a tick every interval:
    1. For each user: QueryUserStats → cursor (settings key `stats_cursor_<username>` = JSON `{up,down}` of the last cumulative values) → `delta = cur - cursor`; **cur < cursor (xray restart) → delta = cur**; update the cursor; SampleInsert(username, now, deltaUp, deltaDown).
    2. OnlineUsers → per-user OnlineIPs → DeviceUpsert(ip, now). Users not in the list are left alone (last_seen ages out).
    3. Enforcement: for each user used = SamplesSince(quota_reset_at) (up+down), onlineIPs from step 2 → users.Decide → apply: rollover → UserUpdate(quota_reset_at=NextBoundary); disable → UserSetDisabled + RemoveUser from all enabled inbounds; enable → UserSetDisabled(false) + AddUser.
    4. SyncMembership (reconcile, self-healing after failures).
    5. PruneSamples(now-90d), PruneDevices(now-30d) — once an hour (not every tick).
  - `RunBackup(ctx, st *store.Store, dataDir string)` — a daily ticker: `VACUUM INTO '<dataDir>/backups/panel-YYYYMMDD.db'`, delete copies older than 7. A backup error → log, no panic.
  - Aggregates for the API: `Traffic(st, username string, rng string) ([]store.Bucket, error)` — day→hour buckets over 24h, week→day over 7d, month→day over 30d; **username="" → across all users in total**; `Summary(st) (totalUp, totalDown int64, onlineCount int, err error)`.

- [ ] **Step 1: Failing tests** (StatsSource/MemberSync fakes from Interfaces)

`TestDeltaNormal` (cursor 100, cur 150 → sample 50); `TestDeltaReset` (cursor 500, cur 30 → sample 30, not -470); `TestCursorSurvivesPanelRestart` (the cursor in settings → a new Poller sees the previous cursor, no duplicated delta); `TestEnforcementIntegration` (fake: a user with quota 100, samples 120 → after the tick disabled(quota) + RemoveUser called; rollover → enable + AddUser); `TestOnlineDevices` (IP upsert, last_seen updated, an absent user untouched); `TestPrune` (old samples/devices deleted, fresh ones intact); `TestTrafficBuckets` (day/week/month ranges).
`backup_test.go`: `TestBackupCreatesAndPrunes` (VACUUM INTO creates the file, 8 old copies → 7 remain).

- [ ] **Step 2: FAIL**
- [ ] **Step 3: Implement**
- [ ] **Step 4: PASS**
- [ ] **Step 5: Commit** `git checkout -b stats-20261120-09 dev && git add -A && git commit -m "feat: stats poller, enforcement wiring, aggregates, sqlite backup"`

---

### Task 10: Router wiring + remaining endpoints + serve

**Branch:** `api-20261120-10`

**Files:**
- Create: `internal/api/router.go`, `internal/api/handlers.go`, `web/embed.go`, `web/dist/index.html` (placeholder «build me»)
- Modify: `cmd/flypanel/main.go` (serve), `internal/auth/handlers.go` (+ ChangePassword)
- Test: `internal/api/router_test.go`

**Interfaces:**
- Consumes: all the previous packages
- Produces:
  - `api.NewRouter(Deps{Store, JWT, RL, Client, Manager, Cfg, Poller}) http.Handler` — stdlib mux: public `POST /api/auth/login|refresh|logout`, `POST /api/auth/2fa/enroll|verify|disable` (enroll/verify/disable are **JWT-protected**: only a logged-in admin changes 2FA; only login/refresh/logout are public), `GET /sub/{token}` and `/sub/{token}/{format}`; everything else under `/api/*` — behind Middleware. SPA: `GET /` and any non-API path → embed index.html (files from embed.FS by path, fallback index.html).
  - Endpoints: `GET /api/xray/status` → Manager.Status JSON; `POST /api/xray/start|stop|restart`; `GET /api/stats/traffic?user=&range=day|week|month`; `GET /api/stats/online` → DevicesOnline(now-2*interval); `GET /api/stats/summary`; `GET/PUT /api/settings` (whitelist: `poll_interval_sec` 5–300; PUT applies the poller interval on the fly); `POST /api/auth/password` {old,new} → VerifyPassword → AdminUpdatePassword + RefreshTokenRevokeAll; `GET /api/users/{id}/sub` → the subscription URL + formats (for the UI copy).
  - `serve`: config.Load → store.Open → seed (if there is no admin — log «run admin init», HTTP still starts; the default inbound + templates + settings if empty) → the JWT key from settings `jwt_key` (generate 32B if absent) → Manager.Start → Poller.Run + RunBackup goroutines → http.Server ListenAddr, graceful shutdown on SIGTERM (5 s).
  - `web/embed.go`: `//go:embed all:dist` → `var Dist embed.FS`.

- [ ] **Step 1: Failing tests**

`TestRouteAuthMatrix` — table (path × cookie → status): `/api/users` without cookie 401 / with a valid one 200; `/api/auth/login` without cookie not 401 (400/401 from the logic, not the middleware); `/api/auth/2fa/enroll` without cookie 401; `/sub/xxx` without cookie not 401 (404 from the logic); `/api/xray/status` without cookie 401.
`TestSPAFallback` (`GET /users` → 200 text/html containing the placeholder; `GET /api/nonexistent` → 404 JSON, not HTML).
`TestSettingsWhitelist` (PUT poll_interval_sec=60 ok; PUT unknown_key → 400; PUT 999 → 400).
`TestPasswordChangeRevokesRefresh`.
`TestXrayStatusEndpoint` (fake manager → JSON fields).

- [ ] **Step 2: FAIL**
- [ ] **Step 3: Implement**
- [ ] **Step 4: PASS** + `go vet ./...`
- [ ] **Step 5: Commit** `git checkout -b api-20261120-10 dev && git add -A && git commit -m "feat: router wiring, settings/xray/stats endpoints, serve, spa embed"`

---

### Task 11: Web scaffold + auth UI

**Branch:** `web-scaffold-20261120-11`

**Files:**
- Create: `web/package.json` (version pins), `web/vite.config.ts`, `web/svelte.config.js`, `web/tsconfig.json`, `web/index.html`, `web/src/main.ts`, `web/src/App.svelte`, `web/src/app.css`, `web/src/lib/api.ts`, `web/src/lib/auth.svelte.ts`, `web/src/routes/Login.svelte`, `web/src/lib/components/` (shadcn-svelte init), `web/vitest.config.ts`
- Test: `web/src/lib/api.test.ts`, `web/src/lib/auth.test.ts`

**Interfaces:**
- Consumes: backend API (Task 10) — dev proxy `/api`,`/sub` → `127.0.0.1:3002`
- Produces:
  - `api.ts`: `apiFetch(path, opts)` — credentials include, JSON; on 401 → one POST /api/auth/refresh → retry once → otherwise redirect to login. `login(username,password,totp?)`, `logout()`, `enroll2FA()`, `verify2FA(code)`, `disable2FA(code)`, `changePassword(old,new)`.
  - `auth.svelte.ts`: a runes store `{loggedIn, need2fa}` + `checkSession()` (GET /api/settings as a probe).
  - App shell: sidebar (Дашборд, Юзеры, Inbounds, Routing, Шаблоны, Настройки, Конфиг), dark theme by default, svelte-spa-router, Russian language. Login: password form → on `2fa_required` a code field (TOTP or backup) → redirect /.
  - Version pins: svelte 5.59.x, vite 8.3.x, @sveltejs/vite-plugin-svelte 7.3.x, tailwindcss 4.3.x, @tailwindcss/vite, shadcn-svelte 1.8.x, svelte-spa-router, qrcode, uplot, vitest. **Check the current versions at start** (`npm view`), pin exact.
- [ ] **Step 1: Failing tests** (vitest, mock fetch): `test('401 → refresh → retry once')`, `test('refresh fail → logout redirect')`, `test('login 2fa_required → need2fa state')`.
- [ ] **Step 2: FAIL** (`npx vitest run`)
- [ ] **Step 3: Implement** scaffold + api + auth + Login + shell. shadcn-svelte init (style default, dark, css variables).
- [ ] **Step 4: PASS** + `npm run build` green.
- [ ] **Step 5: Commit** `git checkout -b web-scaffold-20261120-11 dev && git add -A && git commit -m "feat: web scaffold svelte5+tw4+shadcn, auth ui, api client"`

---

### Task 12: Web — Users, Inbounds, Config viewer

**Branch:** `web-pages-20261120-12`

**Files:**
- Create: `web/src/routes/Users.svelte`, `web/src/routes/Inbounds.svelte`, `web/src/routes/Config.svelte`, `web/src/lib/components/UserDialog.svelte`, `web/src/lib/components/InboundForm.svelte`
- Test: `web/src/routes/Users.test.ts` (vitest + @testing-library/svelte)

**Interfaces:**
- Consumes: api.ts (Task 11), endpoints /api/users, /api/inbounds, /api/config, /api/users/{id}/sub
- Produces: the pages. Users: a table (username, traffic used/quota, online devices, expiry, a disabled_reason status badge), a create/edit dialog (all fields: expiry datetime, quota_bytes, quota_period select none/день/неделя/месяц, device_limit + the NAT hint «один роутер = один IP»), delete confirm, a quota reset button, copy sub URL + 4 formats. Inbounds: list + form (port, path, mode select, dest, serverNames, shortIds, fp) + raw JSON textarea + regenerate keys + enable toggle + delete; the warning «применение = рестарт xray ~1–2 с». Config: GET /api/config pretty-printed readonly + copy.
- [ ] **Step 1: Failing test** `Users.test.ts`: render the table with mock data (2 users, one disabled(quota) → the badge «Квота»), the create button opens the dialog.
- [ ] **Step 2: FAIL**
- [ ] **Step 3: Implement**
- [ ] **Step 4: PASS** + build green.
- [ ] **Step 5: Commit** `git checkout -b web-pages-20261120-12 dev && git add -A && git commit -m "feat: web users, inbounds, config viewer pages"`

---

### Task 13: Web — Dashboard, Routing, Templates, Settings

**Branch:** `web-dashboard-20261120-13`

**Files:**
- Create: `web/src/routes/Dashboard.svelte`, `web/src/routes/Routing.svelte`, `web/src/routes/Templates.svelte`, `web/src/routes/Settings.svelte`, `web/src/lib/components/TrafficChart.svelte` (uPlot), `web/src/lib/traffic.ts` (data transform)
- Test: `web/src/lib/traffic.test.ts`, `web/src/routes/Settings.test.ts`

**Interfaces:**
- Consumes: api.ts, /api/stats/*, /api/xray/*, /api/config/routing, /api/templates/*, /api/settings, /api/auth/2fa/*, /api/auth/password
- Produces: Dashboard — a uPlot traffic chart (day/week/month switch, user/all select), an online devices table (user, IP, last_seen), an xray card (running/version/uptime/restarts + start/stop/restart buttons with confirm), total traffic. Routing — a JSON textarea + a «Проверить» button (JSON.parse) + «Применить» (PUT) + a restart warning. Templates — 3 tabs (json/mihomo/clash) raw textarea + save + a hint of the placeholders `{{UUID}}…`. Settings — password change; 2FA: enroll → QR (`qrcode.toDataURL(otpauthURI)`) + the secret as text + 10 backup codes (show once, «сохрани») → verify with a code → enabled; disable with a code; poll interval.
- [ ] **Step 1: Failing tests**: `traffic.test.ts` (buckets → uPlot series [timestamps, up, down], empty data → zeros); `Settings.test.ts` (enroll mock → the QR data URL generated, 10 backup codes rendered).
- [ ] **Step 2: FAIL**
- [ ] **Step 3: Implement**
- [ ] **Step 4: PASS** + build green.
- [ ] **Step 5: Commit** `git checkout -b web-dashboard-20261120-13 dev && git add -A && git commit -m "feat: web dashboard, routing editor, templates, settings+2fa ui"`

---

### Task 14: Full build + embed + cross-compile + local smoke

**Branch:** `build-20261120-14`

**Files:**
- Modify: `Makefile`, `web/dist/index.html` (the placeholder already exists), `README.md` (build/run instructions, anonymised)
- Create: `scripts/smoke.sh` (local smoke, no real values)
- Test: smoke script

**Interfaces:**
- Consumes: everything
- Produces: `make build` = npm ci → npm run build → go build -o bin/flypanel (SPA inside); `make cross` = a linux/amd64 binary; `scripts/smoke.sh`: bring up the binary with a dev .env (127.0.0.1:3002, XRAY_BIN — a sleep-script stub), create an admin via `flypanel admin init --password-stdin` (pipe), curl login → cookie → GET /api/users 200 → GET / → index.html (the built SPA, not the placeholder) → kill.
- [ ] **Step 1:** `make build` — green, the binary contains the SPA (check: `curl /` serves the built index.html with hashed assets).
- [ ] **Step 2:** `scripts/smoke.sh` — write it (the admin is created via `flypanel admin init --password-stdin`, the password from a pipe), run it, PASS.
- [ ] **Step 3:** `make cross` — `file bin/flypanel-linux-amd64` → ELF 64-bit x86-64.
- [ ] **Step 4:** A grep check for anonymisation across the whole repo against the **denylist from the operator's private runbook** (a file outside the repo, not to be committed): `grep -rInE -f <private-denylist-file> --exclude-dir=.git --exclude-dir=node_modules --exclude-dir=bin .` → **0 matches**. The denylist itself never lands in the repo.
- [ ] **Step 5: Commit** `git checkout -b build-20261120-14 dev && git add -A && git commit -m "feat: full build pipeline, embed spa, cross-compile, smoke"`

---

### Task 15: Deploy + E2E (OPERATOR-INTERACTIVE, dedicated VPS)

**Branch:** no code branch — an ops playbook. **Run in the main session, NOT by a subagent. Every step — the user's explicit "yes".** Real values come from the operator's private runbook, not from the repo. Results go into the private journal (outside the repo). The target server is a dedicated VPS (ADR-0008); the old server with Remnawave is physically untouched.

**Files:**
- Create: `docs/deploy-checklist.md` (an anonymised checklist with placeholders)
- Server artifacts (outside the repo): `/opt/flypanel/.env`, systemd unit, Caddyfile

**Interfaces:**
- Consumes: the linux/amd64 binary (Task 14), spec §8
- Produces: a working prod stack + E2E evidence + journal

- [ ] **Step 1: Ports + swap.** On the VPS (read-only, permission): `ss -ltnp | grep -E ':80|:443|:8443|:3002|:10811'` → empty (don't touch the provider agent's 8080/8189). Occupied → STOP, choose other ports with the operator (update .env, not the code). Swap: `swapon --show` empty → create a 2 GB swapfile (`fallocate -l 2G /swapfile && chmod 600 /swapfile && mkswap /swapfile && swapon /swapfile` + an fstab entry) — a separate "yes".
- [ ] **Step 2: Files.** Create /opt/flypanel/{bin,data,logs,backups}; upload the binary; download xray **v26.11.9** official linux-amd64 release → bin/xray (verify the checksum); `.env` with real values (chmod 600): LISTEN_ADDR=`127.0.0.1:3002`, ENTRY_IP, EGRESS_IP= (empty — one IP), PANEL_DOMAIN, XRAY_BIN, DATA_DIR.
- [ ] **Step 3: systemd.** Unit flypanel.service (User=flypanel, NoNewPrivileges, ProtectSystem=strict, ReadWritePaths=/opt/flypanel, Restart=on-failure); `flypanel admin init` (the operator types the password); start; `journalctl -u flypanel --tail 30` — xray started, the poller is running.
- [ ] **Step 4: UFW.** allow 22/80/443/8443/tcp (world). 3002/10811 — loopback only, no outbound rules needed. Don't change foreign rules (provider agent).
- [ ] **Step 5: DNS + Caddy (a separate "yes"). DNS:** the operator repoints the A record of `<PANEL_DOMAIN>` to `<ENTRY_IP>` (it currently points at the old server); wait for propagation (a DoH check). **Caddy:** native install (official repository) → a Caddyfile from scratch: the block `<PANEL_DOMAIN> { reverse_proxy 127.0.0.1:3002 }` → `caddy validate` → `systemctl reload caddy` (warn: reload tears down established panel connections; there are no other sites on this Caddy, so no quiet window is needed) → runtime verification on :2019 + the LE certificate + `curl https://<PANEL_DOMAIN>/` → the SPA. Back up `cp -a Caddyfile Caddyfile.bak-YYYYMMDD` before edits.
- [ ] **Step 6: E2E of the new channel.** Create a test user via the API/UI. On the server: an xray client with a config from `/sub/<token>/json[0]` → `curl --socks5-hostname 127.0.0.1:<port> https://api.ipify.org` = **`<ENTRY_IP>`** (one IP; if EGRESS_IP is set — that one). All 4 subscription formats serve valid content (base64 decodes, json parses, yaml is valid). The test client is visible in the dashboard (IP + traffic > 0 after polling). Quota: set the test user's quota to 1 MB → after traffic disabled(quota) → reset → enable.
- [ ] **Step 7: Old stack.** A physically separate server — no regression needed. Optionally (a separate "yes", read-only): the old channel is alive, the old subscription serves configs.
- [ ] **Step 8: Rollback (readiness).** `systemctl stop flypanel`; close 8443 in UFW; remove the Caddy block + reload; revert DNS if needed. The /opt/flypanel data is untouched.
- [ ] **Step 9: Journal.** Write into the operator's private journal (outside the repo): what was done, verifications, rollback. In the repo — only the anonymised `docs/deploy-checklist.md`.
- [ ] **Step 10: Commit** `git checkout -b deploy-20261120-15 dev && git add docs/deploy-checklist.md && git commit -m "docs: anonymized deploy checklist"`

---

## Self-review notes

- Spec coverage: §6.1→T4, §6.2→T2/T6/T9, §6.3→T3/T7, §6.4→T3/T7, §6.5→T7/T13, §6.6→T8, §6.7→T5/T9, §6.8→T1/T2/T9, §6.9→T4/T6/T7/T8/T10, §7→T11–T13, §8→T15, §9→branches+tests in every Task, §3→Global Constraints+T14 grep. Non-goals are not implemented.
- Builder decisions (spec §10) recorded by the plan: migrations — embedded SQL + schema_version (no dependency); Reality keygen — Go crypto/ecdh (no subprocess); QR — the `qrcode` npm package; charts — uPlot; JSON editor — textarea+lint (no CodeMirror in v1); frontend router — svelte-spa-router; per-period counter — an aggregate from samples (no counter column); the cumulative stats cursor — in settings (survives a panel restart); samples retention 90d / devices 30d; username immutable; no base64 template (structural links); backup — VACUUM INTO from a goroutine (no cron); JWT key — settings, generated on first start; admin password ≥12 characters; `admin init --password-stdin` for automation.
- Deviations from the spec sketch (deliberate, for the operator to review): (1) `/api/auth/2fa/*` **are JWT-protected** (spec §6.9 says "except /api/auth/*", but a public enroll would let an attacker impose their own 2FA and lock the admin out — only login/refresh/logout are public); (2) a `Subscription-Userinfo` header was added to /sub (a de-facto standard for clients, the spec does not mention it); (3) the anonymisation denylist — a file outside the repo, not committed (the list of real values itself is a leak).
- Amendment 20.11.2026 (ADR-0008, dedicated VPS): bind 127.0.0.1:3002 instead of the docker gateway; native Caddy from scratch (no quiet window/inode traps); EGRESS_IP empty → sendThrough omitted (T3 `TestGenerateNoEgress`); 2 GB swap and the DNS repoint added to T15; no old-stack regression needed (physically separate server); `.gitignore` created in advance at the repo root.