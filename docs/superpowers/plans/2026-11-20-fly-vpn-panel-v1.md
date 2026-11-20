# fly-vpn-panel v1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Один Go-бинарник `flypanel` (SPA внутри) + дочерний xray: полная панель управления VPN (VLESS+XHTTP+Reality) с юзерами, квотами, подписками и видимостью «кто онлайн + сколько трафика».

**Architecture:** Go stdlib net/http + SQLite (WAL) = source of truth; xray v26.11.9 — дочерний процесс, конфиг генерится из БД; структурные правки через `xray run -test` → атомарная запись → рестарт child; юзеры/квоты/expiry — горячо через gRPC AlterInbound; поллер 30 с собирает трафик и онлайн-IP из xray stats API и применяет enforcement. SPA (Svelte 5) вшивается через embed.FS. Деплой — на выделенной VPS (ADR-0008) за поддоменом собственного нативного Caddy, VPN — Reality на своём порту.

**Tech Stack:** Go ≥1.22 (stdlib mux), modernc.org/sqlite v1.62.0, github.com/xtls/xray-core v26.11.9 (gRPC-типы; Go-пин = pseudo-version `v1.260327.1-0.20261109052709-676eaf66f8f0` — module path без `/v26`, literal tag невалиден для Go modules; commit = tag v26.11.9, проверено ls-remote), golang-jwt/jwt v5.3.1, pquerna/otp v1.5.0, golang.org/x/crypto (argon2), google/uuid v1.6.0; Svelte 5.59 + Vite 8.3.8 + @sveltejs/vite-plugin-svelte 7.3.1 + Tailwind 4.3.5 + shadcn-svelte 1.8.1, svelte-spa-router, qrcode (QR), uPlot (графики), vitest.

**Spec:** `docs/superpowers/specs/2026-11-20-fly-vpn-panel-design.md` (одобрен 2026-11-20). ADR: `docs/adr/` 0001–0008 accepted (0001 amended by 0008).

## Global Constraints

- Репо **публичный**: никаких реальных IP/доменов/имён/путей к секретам в любом артефакте (код, тесты, фикстуры, доки). Плейсхолдеры: `<ENTRY_IP>`, `<PANEL_DOMAIN>`, `<EGRESS_IP>` (опциональный; пуст → sendThrough опускается). В фикстурах — TEST-NET адреса (203.0.113.x / 198.51.100.x). Реальные значения — только серверный `/opt/flypanel/.env` (chmod 600, gitignored) + приватный runbook оператора вне репо. Grep-проверка перед каждым push.
- Ветки `<фича>-20261120-NN` от `dev`, слияние в `dev`. Каждый Task = своя ветка (указана в заголовке).
- Go ≥1.22, HTTP-роутер stdlib `net/http` (patterns Go 1.22+). Фреймворк не нужен.
- SQLite DSN: `_journal_mode=WAL&_busy_timeout=5000&_synchronous=FULL` (modernc v1.62.0 распознаёт только `_synchronous`/`_sync`; голый `synchronous=` молча игнорируется — проверено по исходникам драйвера, Task 1). Один писатель. Бэкап ежедневный, 7 копий.
- xray **v26.11.9** (пин; Go-модуль: pseudo-version `v1.260327.1-0.20261109052709-676eaf66f8f0` = exact commit tag'а, module path без `/v26`). API inbound dokodemo-door `127.0.0.1:10811` tag `api`. Policy level 0: `statsUserUplink/statsUserDownlink/statsUserOnline = true`.
- VPN: VLESS+XHTTP+Reality `0.0.0.0:8443`; вход `<ENTRY_IP>`; freedom `sendThrough <EGRESS_IP>` только если EGRESS_IP задан (выделенная VPS: один IP, поле опущено). Reality dest дефолт `www.microsoft.com:443`, path `/xhttp`, mode `stream-up`, fp `chrome`.
- Панель bind: `127.0.0.1:3002` (прод и dev; LISTEN_ADDR из .env). Выделенная VPS (ADR-0008): 1 vCPU / 1 GB / 10 GB, Ubuntu 24.04, один IP (вход=выход), swap 2 GB создать при деплое, Docker не нужен, Caddy нативный. Агент провайдера (8080/8189) не трогать.
- Auth: один админ, CLI `flypanel admin init`, публичной регистрации НЕТ. argon2id. JWT access 15 мин + refresh 7 дней, httpOnly cookies SameSite=Strict, refresh в БД с отзывом. TOTP опционально → обязателен после включения; 10 backup-кодов (хеши, одноразовые); отключение только с действующим кодом. Rate limit логина 5/мин/IP.
- Юзеры: quota_bytes (0=безлимит), quota_period none|day|week|month, **up+down суммарно**, счётчик = агрегат дельт с quota_reset_at; device_limit (дефолт 0) по **concurrent distinct онлайн-IP**, авто-возврат; expiry авто-отключение/включение; disabled_reason quota|device_limit|expired|manual; manual никогда не авто-включается. Membership v1: все активные юзеры во всех включённых inbound.
- Подписки: `https://<PANEL_DOMAIN>/sub/<token>` + `/base64 /json /mihomo /clash`; UA-fallback: v2rayNG→base64, FlClash / clash-verge (lowercase)→mihomo, прочее→base64. Шаблоны raw в БД, **без санитайзера нигде**. Дефолты зеркалят продакшн (read-only чтение разрешено оператором; пути — приватный runbook).
- Поллер gRPC 30 с (конфигурируемо). Счётчики xray сбрасываются при рестарте — потеря ≤ интервала.
- UI: **русский**, тёмная тема. SPA embed.FS. Node ≥20.19/22.12 только на сборке. Версии npm пинить при старте (могли уплыть).
- Серверные действия: только SSH-инструментарий оператора, **явное разрешение на каждое действие**, секреты не печатать. Старый сервер с действующим стеком физически отдельно — НЕ ТРОГАТЬ (единственное касание: read-only чтение прод-шаблонов, Task 8). Результаты — в приватный журнал оператора (вне репо).
- Timestamps в БД и API: RFC3339 **UTC**. Границы периодов квоты — UTC (день 00:00, неделя понедельник 00:00, месяц 1-е 00:00).

## Review Focus

1. **Рестарт xray посреди периода** обнуляет счётчики → дельта не должна уйти в минус или задвоиться; курсор переживает рестарт самой панели. → Task 9 тесты `TestDeltaReset`, `TestCursorSurvivesPanelRestart`.
2. **Rollover периода у отключённого по квоте юзера** → авто-включение ровно на границе (UTC-края месяца/недели). → Task 6 `TestNextBoundary`, `TestDecideRolloverEnables`.
3. **Невалидный raw-конфиг/routing** → старый конфиг нетронут, частичной записи нет, рестарта нет. → Task 7 `TestApplyAtomicFailureKeepsOld`, `TestInvalidRouting422`.
4. **Auth-края**: просроченный/подделанный JWT, повторное использование backup-кода, 6-я попытка логина за минуту, логин без TOTP когда он включён. → Task 4 соответствующие тесты.
5. **Подписки**: неизвестный токен → 404 без утечки существования юзера; expired/disabled → 403 + горячее отключение; UA `ClashVerge/*` (CamelCase) → base64 (не сломанный clash). → Task 8 `TestSubAccess`, `TestFormatForUA`.

---

### Task 1: Scaffold + config + SQLite core

**Branch:** `store-20261120-01`

**Files:**
- Create: `go.mod`, `Makefile`, `.env.example`, `cmd/flypanel/main.go`, `internal/config/config.go`, `internal/store/store.go`, `internal/store/migrations.go`, `internal/store/migrations/0001_init.sql`
- Test: `internal/config/config_test.go`, `internal/store/store_test.go`

**Interfaces:**
- Consumes: —
- Produces:
  - `config.Load() (*Config, error)` — читает `.env` (путь из `FLYPANEL_ENV`, дефолт `./.env`; отсутствие файла OK). Поля: `DataDir string` (дефолт `./data`), `ListenAddr string` (дефолт `127.0.0.1:3002`), `XrayBin string` (дефолт `./bin/xray`), `EntryIP, EgressIP, PanelDomain string` (из .env, в dev могут быть пустыми), `PollIntervalSec int` (дефолт 30).
  - `store.Open(dataDir string) (*Store, error)` — создаёт dataDir, открывает `panel.db` с DSN-прагмами, применяет миграции. `(*Store).DB() *sql.DB`.
  - Миграции: embed `migrations/*.sql`, таблица `schema_version(version INTEGER)`, применение в транзакции по порядку, идемпотентно.

Схема `0001_init.sql` (все таблицы v1, timestamps TEXT RFC3339 UTC):
- `admin(id INTEGER PK CHECK(id=1), username TEXT UNIQUE NOT NULL, password_hash TEXT NOT NULL, totp_secret TEXT, totp_enabled INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL)`
- `backup_codes(id INTEGER PK, code_hash TEXT NOT NULL, used INTEGER NOT NULL DEFAULT 0)`
- `users(id INTEGER PK, username TEXT UNIQUE NOT NULL, uuid TEXT NOT NULL, sub_token TEXT UNIQUE NOT NULL, expiry TEXT NOT NULL, quota_bytes INTEGER NOT NULL DEFAULT 0, quota_period TEXT NOT NULL DEFAULT 'none' CHECK(quota_period IN ('none','day','week','month')), quota_reset_at TEXT NOT NULL, device_limit INTEGER NOT NULL DEFAULT 0, disabled INTEGER NOT NULL DEFAULT 0, disabled_reason TEXT, created_at TEXT NOT NULL)`
- `inbounds(id INTEGER PK, tag TEXT UNIQUE NOT NULL, enabled INTEGER NOT NULL DEFAULT 1, config_json TEXT NOT NULL, created_at TEXT NOT NULL)` — `config_json` = полный объект inbound без clients (clients инжектятся при генерации); port живёт внутри config_json.
- `settings(key TEXT PK, value TEXT NOT NULL)`
- `traffic_samples(id INTEGER PK, username TEXT NOT NULL, ts TEXT NOT NULL, up_delta INTEGER NOT NULL, down_delta INTEGER NOT NULL)` + индекс `(username, ts)`
- `devices(id INTEGER PK, username TEXT NOT NULL, ip TEXT NOT NULL, first_seen TEXT NOT NULL, last_seen TEXT NOT NULL, UNIQUE(username, ip))`
- `refresh_tokens(id INTEGER PK, token_hash TEXT UNIQUE NOT NULL, expires_at TEXT NOT NULL, revoked INTEGER NOT NULL DEFAULT 0)`

`.gitignore` уже создан в корне репо (секреты/БД/бинарники/логи/бэкапы/node_modules/dist, исключение `!web/dist/index.html` для embed-placeholder) — проверить, дополнить при необходимости. `.env.example` — только плейсхолдеры, EGRESS_IP пустой по умолчанию. `Makefile`: `build` (npm ci+build → go build), `test` (go test ./...), `web-test`, `cross` (GOOS=linux GOARCH=amd64). `main.go` пока: подкоманда `version`, заглушка `serve` (полноценный serve — Task 10).

- [ ] **Step 1: Падающие тесты**

`config_test.go`: `TestLoadDefaults` (пустой env → DataDir `./data`, ListenAddr `127.0.0.1:3002`, PollIntervalSec 30, EgressIP пуст); `TestLoadEnvFile` (временный .env с `LISTEN_ADDR=203.0.113.1:3002`, `EGRESS_IP=198.51.100.7` → поля совпадают).
`store_test.go`: `TestOpenMigrates` (Open во временный dir → `schema_version`=1, все 8 таблиц существуют); `TestOpenIdempotent` (повторный Open не падает, версия та же); `TestWALPragmas` (`PRAGMA journal_mode` → `wal`, `PRAGMA busy_timeout` → 5000, `PRAGMA synchronous` → 2).

- [ ] **Step 2: Прогнать — FAIL** (`go test ./...`: пакеты не компилируются)
- [ ] **Step 3: Реализовать** config (парсер KEY=VALUE, без зависимости), store.Open + migrations runner. Точные DSN-прагмы из Global Constraints.
- [ ] **Step 4: Прогнать — PASS** (`go test ./...`)
- [ ] **Step 5: Commit**

```bash
git checkout -b store-20261120-01 dev
git add -A && git commit -m "feat: scaffold, config, sqlite core + schema v1"
```

---

### Task 2: Store queries (все сущности)

**Branch:** `store-20261120-02`

**Files:**
- Create: `internal/store/admin.go`, `internal/store/users.go`, `internal/store/inbounds.go`, `internal/store/settings.go`, `internal/store/samples.go`, `internal/store/devices.go`, `internal/store/tokens.go`, `internal/store/types.go`
- Test: `internal/store/queries_test.go`

**Interfaces:**
- Consumes: `store.Store` / `DB()` (Task 1)
- Produces (методы `*Store`; типы в `types.go`: `Admin{ID, Username, PasswordHash, TOTPSecret string, TOTPEnabled bool, CreatedAt time.Time}`, `User{ID int64, Username, UUID, SubToken string, Expiry time.Time, QuotaBytes int64, QuotaPeriod string, QuotaResetAt time.Time, DeviceLimit int, Disabled bool, DisabledReason string, CreatedAt time.Time}`, `Inbound{ID int64, Tag string, Enabled bool, ConfigJSON string, CreatedAt time.Time}`, `Device{Username, IP string, FirstSeen, LastSeen time.Time}`, `Bucket{TS time.Time, Up, Down int64}`):
  - `AdminGet() (*Admin, error)` (ErrNotFound если нет); `AdminCreate(username, hash string) error` (ошибка если админ уже есть); `AdminUpdatePassword(hash string) error`; `AdminSetTOTP(secret string, enabled bool) error`
  - `BackupCodesReplace(hashes []string) error`; `BackupCodeConsume(hash string) (bool, error)` — атомарно used=1, false если уже used/нет
  - `UserCreate(*User) error`; `UserGet(id int64)`; `UserGetByToken(token string)`; `UserGetByUsername(name string)`; `UserList() ([]User, error)`; `UserUpdate(*User) error`; `UserDelete(id int64) error`; `UserSetDisabled(id int64, disabled bool, reason string) error` (reason="" при включении)
  - `InboundCreate/Get/List/Update/Delete` (той же формы); `InboundListEnabled() ([]Inbound, error)`
  - `SettingGet(key) (string, error)` (ErrNotFound); `SettingSet(key, value string) error`; `SettingsAll() (map[string]string, error)`
  - `SampleInsert(username string, ts time.Time, up, down int64) error`; `SamplesSince(username string, since time.Time) (up, down int64, err error)` — SUM; `TrafficSeries(username string, from, to time.Time, bucket string) ([]Bucket, error)` — bucket `hour|day`, GROUP BY; `PruneSamples(before time.Time) error`
  - `DeviceUpsert(username, ip string, now time.Time) error` — INSERT … ON CONFLICT(username,ip) UPDATE last_seen; `DevicesOnline(since time.Time) ([]Device, error)`; `DevicesForUser(username string) ([]Device, error)`; `PruneDevices(before time.Time) error`
  - `RefreshTokenInsert(hash string, exp time.Time) error`; `RefreshTokenValid(hash string) (bool, error)` — не revoked и не истёк; `RefreshTokenRevoke(hash string) error`; `RefreshTokenRevokeAll() error`
  - `var ErrNotFound = errors.New("not found")`

- [ ] **Step 1: Падающие тесты** — roundtrip на каждую сущность + края: `TestUserCRUD` (create→get→update→list→delete; duplicate username → ошибка); `TestUserSetDisabled` (reason пишется/очищается); `TestBackupCodeOneTime` (consume true → повторно false); `TestAdminSingle` (второй AdminCreate → ошибка); `TestSamplesSince` (3 sample, since отсекает, SUM up+down раздельно); `TestTrafficSeries` (бакеты hour/day агрегируют); `TestDeviceUpsert` (first_seen не меняется, last_seen обновляется); `TestRefreshTokenLifecycle` (valid → revoke → invalid; истёкший → invalid); `TestSettingsRoundtrip`.
- [ ] **Step 2: FAIL**
- [ ] **Step 3: Реализовать** — database/sql, без ORM. Время: `time.Time.UTC().Format(time.RFC3339)` / parse.
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
  - `GenerateConfig(inbounds []store.Inbound, activeUsers []store.User, routingJSON, egressIP string, apiPort int) ([]byte, error)` — полный конфиг: `log{loglevel:"warning"}`; `api{tag:"api",services:["HandlerService","StatsService"]}`; `policy{levels:{"0":{statsUserUplink:true,statsUserDownlink:true,statsUserOnline:true}},system:{statsInboundUplink:true,statsInboundDownlink:true}}`; inbounds: dokodemo-door `127.0.0.1:apiPort` tag `api` (sockopt none) + каждый enabled inbound из `ConfigJSON` с `settings.clients = [{id: uuid, email: username, level: 0} …]`; outbounds: `freedom` (tag `direct`, `sendThrough: egressIP` если непустой) + `blackhole` tag `block`; `routing` = распарсенный routingJSON (дефолт `{"domainStrategy":"AsIs","rules":[]}`). Пустой routingJSON → дефолт. Невалидный JSON → ошибка.
  - `Validate(binPath string, cfg []byte) error` — temp-файл, `exec.CommandContext(binPath, "run", "-test", "-c", tmp)`, timeout 10 с, stderr в текст ошибки.
  - `ApplyAtomic(path string, cfg []byte) error` — temp в том же dir + `os.Rename`.
  - `NewManager(binPath, cfgPath string) *Manager`; `(*Manager).Start() error` / `Stop() error` / `Restart() error`; `Status() Status` где `Status{Running bool, PID int, Uptime time.Duration, Restarts int, Version string}`. Крах процесса → авто-рестарт backoff 1s→2s→4s… cap 30s, счётчик Restarts. Stop — graceful (SIGTERM, 5 с, затем Kill). Version — `xray version` при старте (первая строка).
  - `NewKeyPair() (private, public string, err error)` — `crypto/ecdh.X25519().GenerateKey(rand.Reader)`, base64.RawURLEncoding (формат xray, 43 символа).

- [ ] **Step 1: Падающие тесты**

`config_test.go`: `TestGenerateGolden` — 1 enabled inbound (fixture: port 8443, Reality dest `www.microsoft.com:443`, pbk/sid тестовые, path `/xhttp`, mode stream-up) + 2 активных юзера + routing с правилом `{"type":"field","domain":["geosite:category-ads-all"],"outboundTag":"block"}` + egressIP `198.51.100.7` → JSON семантически равен `testdata/golden.json` (сравнение через unmarshal+reflect, порядок ключей не важен). `TestGenerateInjectsOnlyActive` — disabled юзер не попадает в clients. `TestGenerateNoSanitizer` — routing с domain/ip правилами проходит дословно. `TestGenerateEmptyRouting` → дефолт. `TestGenerateBadRouting` → ошибка. `TestGenerateNoEgress` — egressIP `""` → freedom без поля sendThrough (прод-дефолт выделенной VPS: один IP).
`keys_test.go`: `TestKeyPairFormat` (43 chars, RawURL base64, pub детерминированно выводится из priv через ecdh).
`proc_test.go`: `TestManagerStartStop` (фейковый «xray» — shell-скрипт `sleep infinity` во временном dir; Start → Running, Stop → не Running); `TestManagerCrashBackoff` (скрипт `exit 1` → Restarts растёт, процесс не зомби); `TestApplyAtomicFailureKeepsOld` (rename в несуществующий dir → ошибка, старый файл цел). Validate с реальным бинарником — integration за `XRAY_BIN` env (skip если нет): валидный golden → nil, битый JSON → ошибка.

- [ ] **Step 2: FAIL**
- [ ] **Step 3: Реализовать.** Manager: один mutex, cmd.Process, wait-горутина с backoff. Генератор — `map[string]any` + json.MarshalIndent.
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
  - `HashPassword(pw string) (string, error)` / `VerifyPassword(hash, pw string) bool` — argon2id time=1 memory=64MiB threads=4 keyLen=32 saltLen=16, PHC-строка `$argon2id$v=19$m=65536,t=1,p=4$…`.
  - `NewJWT(key []byte) *JWT`; `(*JWT).IssueAccess() (string, error)` — claims `{sub:"admin", exp: now+15m}`, HS256; `(*JWT).VerifyAccess(token string) error`; `IssueRefresh() (raw, hash string, err error)` — 32 случайных байта base64url, hash=sha256; refresh exp 7д хранится в БД (Task 2).
  - `EnrollTOTP(account string) (secret, otpauthURI string, err error)` — pquerna/otp totp.Generate, issuer `flypanel`; `ValidateTOTP(secret, code string) bool` — totp.Validate (skew 1); `GenerateBackupCodes() (codes, hashes []string, err error)` — 10 кодов по 8 символов base32, hash=sha256.
  - `NewRateLimiter(limit int, window time.Duration) *RateLimiter`; `Allow(ip string) bool` — in-memory sliding window.
  - `Middleware(j *JWT) func(http.Handler) http.Handler` — access cookie `fp_access`; нет/невалид → 401 JSON `{"error":"unauthorized"}`.
  - `Handlers{Store *store.Store, JWT *JWT, RL *RateLimiter}` с методами `Login/Refresh/Logout/Enroll2FA/Verify2FA/Disable2FA http.HandlerFunc`. Cookie-имена: `fp_access` (15m), `fp_refresh` (7d, path `/api/auth`), httpOnly SameSite=Strict.
  - CLI `flypanel admin init`: интерактивно username + пароль (≥12 символов, подтверждение; `golang.org/x/term.ReadPassword`), ошибка если админ существует. Флаг `--password-stdin` — читать пароль из stdin без подтверждения (для автоматизации/smoke).

Поведение Login: rate limit по IP (5/мин) → 429; неверный пароль → 401 `{"error":"invalid_credentials"}`; 2FA включён и код не передан/невалиден → 401 `{"error":"2fa_required"}`; код = TOTP **или** backup-код (consume); успех → cookies + `{"ok":true}`. Enroll2FA: генерит секрет+URI+10 кодов, сохраняет секрет (totp_enabled остаётся 0), коды возвращает ОДИН раз; Verify2FA `{code}` валиден → totp_enabled=1 + BackupCodesReplace; Disable2FA `{code}` валиден → totp_enabled=0, секрет очистить. Refresh: валидный refresh cookie → новый access (refresh не ротируем v1); Logout: revoke refresh + очистить cookies. Смена пароля (Settings, Task 10) → RefreshTokenRevokeAll.

- [ ] **Step 1: Падающие тесты**

`auth_test.go`: `TestArgon2Roundtrip` (+неверный пароль false); `TestJWTExpired` (токен с exp в прошлом → ошибка), `TestJWTTampered` (подпись битая → ошибка); `TestTOTPRFC6238` (секрет `GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ`, время Unix 59, SHA1/6 цифр → код `287082` валиден через ValidateTOTP с фиксацией времени — pquerna totp.ValidateCustom); `TestBackupCodes` (10 штук, 8 символов); `TestRateLimiter` (5 allow, 6-й deny, после окна allow).
`handlers_test.go` (httptest + store во временном dir): `TestLoginFlow` (admin init через AdminCreate+HashPassword → login ok cookies; неверный пароль 401); `TestLogin2FARequired` (включён 2FA → без кода 401 `2fa_required`, с валидным TOTP ok, backup-код ok и повторно 401); `TestLoginRateLimit` (6 запросов → 429); `TestRefreshLogout` (refresh даёт access; logout → refresh невалиден); `TestMiddleware` (без cookie 401, с валидным 200, просроченный 401); `TestAdminInitSecondFails`.

- [ ] **Step 2: FAIL**
- [ ] **Step 3: Реализовать**
- [ ] **Step 4: PASS**
- [ ] **Step 5: Commit** `git checkout -b auth-20261120-04 dev && git add -A && git commit -m "feat: auth argon2id, jwt, totp, backup codes, rate limit, admin cli"`

---

### Task 5: gRPC client xray (stats + handler)

**Branch:** `xray-grpc-20261120-05`

**Files:**
- Create: `internal/xrayclient/client.go`, `internal/xrayclient/fake_test.go`
- Test: `internal/xrayclient/client_test.go`
- Modify: `go.mod` (+ `github.com/xtls/xray-core` pseudo-version `v1.260327.1-0.20261109052709-676eaf66f8f0` = tag v26.11.9, `google.golang.org/grpc`)

**Interfaces:**
- Consumes: xray-core модуль (типы `app/stats/command`, `app/proxyman/command`), конфиг с api inbound (Task 3)
- Produces:
  - `New(addr string) (*Client, error)` — grpc.NewClient insecure (loopback).
  - `(*Client).QueryUserStats(email string) (up, down int64, err error)` — QueryStats паттерны `user>>><email>>>traffic>>>uplink` / `…downlink` (3 стрелки — проверено по исходникам v26.11.9, T5), `Reset_: false`; отсутствующий счётчик → 0.
  - `(*Client).OnlineUsers() ([]string, error)` — GetAllOnlineUsers.
  - `(*Client).OnlineIPs(email string) ([]string, error)` — GetStatsOnlineIpList.
  - `(*Client).AddUser(inboundTag, uuid, email string) error` / `RemoveUser(inboundTag, email string) error` — AlterInbound AddUserOperation/RemoveUserOperation (user level 0); ошибка «already exists» → nil (идемпотентность).
  - `(*Client).Close() error`.

- [ ] **Step 1: Проверить семантику (spec §6.7).** Прочитать исходники xray-core v26.11.9 (`app/stats/command/command.go`, `app/proxyman/command/command.go`): подтвердить что GetStatsOnlineIpList возвращает IP по юзеру и GetAllOnlineUsers — список email онлайн; зафиксировать вывод в commit message. Если методы не дают IP — СТОП, сообщить оператору (fallback access-log — отдельное решение, не молча). **Проверено (T5): PASSED** — GetStatsOnlineIpList возвращает per-user IP (map `user>>>email>>>online`), GetAllOnlineUsers — online emails; источник: module cache v26.11.9.
- [ ] **Step 2: Падающие тесты** — bufconn fake-сервер (`fake_test.go` реализует StatsServiceServer + HandlerServiceServer): `TestQueryUserStats` (паттерны точные, reset=false, up/down маппятся; отсутствующий → 0); `TestOnlineUsers`; `TestOnlineIPs`; `TestAddRemoveUser` (AlterInbound получает верный tag/operation; already-exists → nil).
- [ ] **Step 3: FAIL**
- [ ] **Step 4: Реализовать**
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
  - `Decide(u store.User, usedBytes int64, onlineIPs []string, now time.Time) []Action` — чистая функция. Правила (порядок): (1) `expiry <= now` и !disabled → disable(expired); (2) period != none и `now >= NextBoundary(u.QuotaResetAt, period)` → rollover (**при rollover used для quota-проверки = 0** — новый период только начался); (3) quota_bytes>0 и used>=quota_bytes и !disabled → disable(quota); (4) disabled по quota и (после rollover used<quota_bytes) → enable; (5) device_limit>0 и `len(unique(onlineIPs)) > device_limit` и !disabled → disable(device_limit); (6) disabled по device_limit и len<=limit → enable; (7) disabled по expired и expiry>now → enable. **manual — никогда не авто-включается.** Несколько причин одновременно: disable побеждает enable; приоритет причин expiry > quota > device_limit.
  - `NextBoundary(start time.Time, period string) time.Time` — UTC: day → следующие 00:00; week → следующий понедельник 00:00; month → следующее 1-е 00:00; none → start (не меняется).
  - `type MemberSync interface { AddUser(inboundTag, uuid, email string) error; RemoveUser(inboundTag, email string) error }` (удовлетворяется `*xrayclient.Client`).
  - `SyncMembership(ctx context.Context, c MemberSync, inbounds []store.Inbound, active []store.User) error` — reconcile: для каждого enabled inbound добавить всех активных (идемпотентно), удалить тех кого нет в active (по email). Ошибка одного inbound не прерывает остальные (собирается errors.Join).
  - `Handlers{Store, Client, Manager}` — `/api/users` GET/POST, `/api/users/{id}` GET/PATCH/DELETE, `/api/users/{id}/reset-quota` POST. GET list обогащает каждого юзера: `used_bytes` (SamplesSince(quota_reset_at), up+down) и `online_count` (DevicesForUser с last_seen ≥ now-2*interval) — для таблицы UI. POST: uuid v4, sub_token 16 символов base62 crypto/rand, quota_reset_at = начало текущего периода (для day — сегодня 00:00 UTC), expiry обязателен. PATCH: поля username **неизменяем** (ключ статистики); quota_bytes/quota_period/expiry/device_limit/disabled(manual) редактируемы; смена quota_period → quota_reset_at = начало текущего периода; ручное повышение лимита/сброс → если disabled_reason=quota → enable. DELETE → RemoveUser из всех enabled inbound + UserDelete. Любое изменение disabled/expiry → немедленный горячий sync (не ждать поллера).

- [ ] **Step 1: Падающие тесты**

`enforce_test.go` — table-driven `TestDecide` (все переходы): quota exceeded→disable(quota); rollover+used<limit→enable; device over→disable; device back→enable; expiry passed→disable(expired); extended→enable; manual disabled + все условия ok → НЕТ enable; expiry+quota одновременно → disable(expired) приоритет; NAT: 5 соединений с одного IP = 1 distinct (onlineIPs уже distinct от API — тест что Decide считает len, не дубли). `TestNextBoundary`: vectors — day `2026-11-20T15:00Z`→`2026-11-21T00:00Z`; week четверг→понедельник; month `2026-11-20`→`2026-12-01T00:00Z`; `2026-12-31`→`2027-01-01T00:00Z`; none→без изменений. `TestSyncIdempotent` (fake MemberSync: повторный add не ошибка, remove отсутствующего не ошибка).
`handlers_test.go`: `TestUserCreateDefaults` (uuid формат, token 16, device_limit 0, quota_period none); `TestUserListEnriched` (used_bytes = сумма samples с reset_at, online_count по свежим devices); `TestUserPatchQuotaRaiseEnables`; `TestUserDeleteRemovesFromInbounds`; `TestUsernameImmutable` (PATCH username → 400).

- [ ] **Step 2: FAIL**
- [ ] **Step 3: Реализовать**
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
  - `type Applier struct { Store *store.Store; Cfg *config.Config; Mgr *xray.Manager; Validate func(binPath string, cfg []byte) error }` — `NewApplier(...)` ставит `Validate = xray.Validate`; в тестах инжектится фейк. Метод `Apply(routingJSON string) error` — pipeline: GenerateConfig(enabled inbounds, active users, routing, Cfg.EgressIP, 10811) → Validate(Cfg.XrayBin, …) → ApplyAtomic(Cfg.DataDir+"/xray.json") → Mgr.Restart. Ошибка Validate → вернуть ошибку, файл НЕ тронут, рестарта НЕТ.
  - Handlers: `/api/inbounds` GET/POST, `/api/inbounds/{id}` GET/PATCH/DELETE, `/api/inbounds/{id}/keys` POST (NewKeyPair → обновить privateKey/publicKey в config_json → Apply). POST/PATCH/DELETE inbound → Apply. Валидация формы: port 1–65535, tag уникален, Reality dest непустой.
  - `/api/config` GET → текущий файл xray.json как есть; `/api/config/routing` GET → settings key `routing` (дефолт `{"domainStrategy":"AsIs","rules":[]}`); PUT → JSON-parse проверка (только синтаксис, **никакого санитайзера**) → SettingSet → Apply. Провал Apply → 422 `{"error":…}`, старое значение routing сохранено (откат SettingSet).
  - Дефолтный inbound при первой установке (seed в Task 10): tag `vless-xhttp`, port 8443, path `/xhttp`, mode stream-up, dest `www.microsoft.com:443`, serverNames `[dest-host]`, fp chrome, shortIds `[""]`→генерить 8-hex при seed, ключи NewKeyPair.

- [ ] **Step 1: Падающие тесты**

`apply_test.go`: `TestApplyInvalidKeepsOld` (фейковый Validate возвращает ошибку → старый файл байт-в-байт, Manager.Restart не вызван — spy); `TestApplyValidRestarts` (валидный → файл обновлён, restart вызван 1 раз); `TestApplyAtomicFailureKeepsOld`.
`handlers_test.go`: `TestRoutingNoSanitizer` (PUT routing с правилами domain+ip+geoip → GET возвращает ДОСЛОВНО тот же JSON); `TestInvalidRouting422` (битый JSON → 422, старое значение цело); `TestInboundCRUD` (create→list→patch port→delete; duplicate tag → 400); `TestKeysRegen` (POST keys → новая пара 43 chars, отличается от старой).

- [ ] **Step 2: FAIL**
- [ ] **Step 3: Реализовать**
- [ ] **Step 4: PASS**
- [ ] **Step 5: Commit** `git checkout -b inbounds-20261120-07 dev && git add -A && git commit -m "feat: inbounds api, config apply pipeline, routing editor backend"`

---

### Task 8: Subscription generators + templates + /sub handlers

**Branch:** `subs-20261120-08`

**Files:**
- Create: `internal/sub/links.go`, `internal/sub/json.go`, `internal/sub/yaml.go`, `internal/sub/ua.go`, `internal/sub/handlers.go`, `internal/sub/defaults/` (шаблоны), `internal/sub/defaults.go` (embed)
- Test: `internal/sub/sub_test.go`

**Interfaces:**
- Consumes: store, config.Config (EntryIP, PanelDomain)
- Produces:
  - `RenderBase64(inbounds []store.Inbound, u store.User, entryIP string) string` — по строке на enabled inbound: `vless://<uuid>@<entryIP>:<port>?type=xhttp&path=<path>&mode=<mode>&security=reality&pbk=<pub>&sid=<первый shortId>&sni=<первый serverName>&fp=<fp>&encryption=none#<tag>` (query url-encoded), результат base64.StdEncoding от joined `\n`.
  - `RenderJSON(inbounds, u, entryIP, templateJSON string) ([]byte, error)` — templateJSON = клиентский конфиг с плейсхолдерами `{{UUID}} {{ADDRESS}} {{PORT}} {{PBK}} {{SID}} {{SNI}} {{FP}} {{PATH}} {{MODE}} {{TAG}}`; подстановка на каждый enabled inbound → **массив** конфигов. Остаток плейсхолдера после подстановки → ошибка.
  - `RenderYAML(template string, params map[string]string) (string, error)` — та же подстановка `{{KEY}}` для mihomo/clash (текст, yaml не парсим — raw).
  - `FormatForUA(ua string) string` — lowered содержит `v2rayng` → base64; lowered содержит `flclash` → mihomo; **raw** содержит `clash-verge` → mihomo; иначе base64. (ClashVerge CamelCase → base64.)
  - Handlers `GET /sub/{token}` (UA-fallback) и `GET /sub/{token}/{format}` (base64|json|mihomo|clash). Неизвестный токен → 404 `{"error":"not found"}` (единообразно). Юзер expired → UserSetDisabled(expired) + 403 `{"error":"expired"}`; disabled → 403 `{"error":"disabled"}`. Content-Type: base64 text/plain, json application/json, yaml text/yaml. Заголовок `Subscription-Userinfo: upload=…; download=…; total=…; expire=…` (байты из samples, total=quota_bytes, expire=unix expiry) — совместимость с клиентами.
  - Шаблоны: settings keys `template_json`, `template_mihomo`, `template_clash` (base64 — структурный генератор ссылок, шаблона нет). `/api/templates/{format}` GET/PUT (raw, без санитайзера; PUT проверяет только что плейсхолдеры не сломаны рендер-тестом на фиктивном юзере). Seed дефолтов при первой установке (Task 10).
  - **Дефолтные шаблоны зеркалят продакшн** (spec §6.6): gated-шаг ниже.

- [ ] **Step 1: GATED — чтение продакшн-шаблонов (read-only).** Выполняет оператор / основная сессия через SSH-инструментарий, **явное «да» на каждое действие**, пути из приватного runbook (не в репо). Прочитать текущие mihomo/clash/xray-json шаблоны действующей панели. Проверить обезличивание (grep по реальным IP/доменам/именам — в шаблонах их быть не должно, только плейсхолдеры панели-генератора). Адаптировать плейсхолдеры под наш формат `{{…}}`, сохранить в `internal/sub/defaults/`. **Дифф-верификация**: правила RU-direct, adblock (anti-ad.net + 10 Google/YouTube доменов), sniffer, `ipv6: false`, Telegram CIDR → proxy no-resolve — присутствуют дословно. Если продакшн недоступен — СТОП, спросить оператора (не выдумывать дефолты молча).
- [ ] **Step 2: Падающие тесты**

`TestRenderBase64` — декодировать, проверить КАЖДЫЙ query-параметр ссылки (type=xhttp, security=reality, pbk, sid, sni, fp, encryption=none, path, mode), host=entryIP, fragment=tag; 2 enabled inbound → 2 строки.
`TestRenderJSONArray` — 2 inbound → массив len 2, uuid/address/port подставлены, routing из шаблона сохранён дословно (anti-sanitizer).
`TestRenderYAML` — подстановка всех `{{…}}`, остаток `{{` → ошибка; rules/sniffer/ipv6 из дефолта на месте.
`TestFormatForUA` — table: `v2rayNG/1.9.30`→base64, `FlClash/0.9.3 clash-verge`→mihomo, `clash-verge/2.5.10`→mihomo, `ClashVerge/2.2.3`→base64, `clash.meta`→base64, `Mozilla/5.0`→base64, пустой→base64.
`TestSubAccess` (httptest): неизвестный токен → 404 (тело идентично для несуществующего и удалённого юзера); expired → 403 + юзер disabled(expired) в БД; disabled → 403; активный + суффикс /json → валидный массив; базовый URL с UA FlClash → yaml.
`TestTemplatePutNoSanitizer` — PUT mihomo шаблона с DOMAIN-SUFFIX/IP-CIDR правилами → GET дословно.

- [ ] **Step 3: FAIL**
- [ ] **Step 4: Реализовать**
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
  - `type StatsSource interface { QueryUserStats(email string) (up, down int64, err error); OnlineUsers() ([]string, error); OnlineIPs(email string) ([]string, error) }` и `type MemberSync interface { AddUser(tag, uuid, email string) error; RemoveUser(tag, email string) error }` (оба удовлетворяются `*xrayclient.Client`; в тестах — фейки).
  - `NewPoller(st *store.Store, src StatsSource, sync MemberSync, interval time.Duration) *Poller`; `(*Poller).SetInterval(d time.Duration)` — смена интервала на лету (из settings PUT); `Run(ctx)` — тик каждый interval:
    1. Для каждого юзера: QueryUserStats → курсор (settings key `stats_cursor_<username>` = JSON `{up,down}` последних кумулятивных) → `delta = cur - cursor`; **cur < cursor (рестарт xray) → delta = cur**; курсор обновить; SampleInsert(username, now, deltaUp, deltaDown).
    2. OnlineUsers → per-user OnlineIPs → DeviceUpsert(ip, now). Юзеры не в списке — не трогать (last_seen стареет).
    3. Enforcement: для каждого юзера used = SamplesSince(quota_reset_at) (up+down), onlineIPs из шага 2 → users.Decide → применить: rollover → UserUpdate(quota_reset_at=NextBoundary); disable → UserSetDisabled + RemoveUser из всех enabled inbound; enable → UserSetDisabled(false) + AddUser.
    4. SyncMembership (reconcile, self-healing после сбоев).
    5. PruneSamples(now-90d), PruneDevices(now-30d) — раз в час (не каждый тик).
  - `RunBackup(ctx, st *store.Store, dataDir string)` — ежедневный тикер: `VACUUM INTO '<dataDir>/backups/panel-YYYYMMDD.db'`, удалить копии старше 7. Ошибка бэкапа → лог, не паника.
  - Агрегаты для API: `Traffic(st, username string, rng string) ([]store.Bucket, error)` — day→hour buckets за 24ч, week→day за 7д, month→day за 30д; **username="" → по всем юзерам суммарно**; `Summary(st) (totalUp, totalDown int64, onlineCount int, err error)`.

- [ ] **Step 1: Падающие тесты** (фейки StatsSource/MemberSync из Interfaces)

`TestDeltaNormal` (cursor 100, cur 150 → sample 50); `TestDeltaReset` (cursor 500, cur 30 → sample 30, не -470); `TestCursorSurvivesPanelRestart` (cursor в settings → новый Poller видит прежний курсор, delta не задваивается); `TestEnforcementIntegration` (fake: юзер quota 100, samples 120 → после тика disabled(quota) + RemoveUser вызван; rollover → enable + AddUser); `TestOnlineDevices` (IP upsert, last_seen обновлён, отсутствующий юзер не тронут); `TestPrune` (старые samples/devices удалены, свежие целы); `TestTrafficBuckets` (day/week/month диапазоны).
`backup_test.go`: `TestBackupCreatesAndPrunes` (VACUUM INTO создаёт файл, 8 старых копий → остаётся 7).

- [ ] **Step 2: FAIL**
- [ ] **Step 3: Реализовать**
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
- Consumes: все предыдущие пакеты
- Produces:
  - `api.NewRouter(Deps{Store, JWT, RL, Client, Manager, Cfg, Poller}) http.Handler` — stdlib mux: публичные `POST /api/auth/login|refresh|logout`, `POST /api/auth/2fa/enroll|verify|disable` (enroll/verify/disable — **защищены** JWT: только залогиненный админ меняет 2FA; публичны только login/refresh/logout), `GET /sub/{token}` и `/sub/{token}/{format}`; всё остальное `/api/*` — за Middleware. SPA: `GET /` и любой не-API путь → embed index.html (файлы из embed.FS по пути, fallback index.html).
  - Endpoints: `GET /api/xray/status` → Manager.Status JSON; `POST /api/xray/start|stop|restart`; `GET /api/stats/traffic?user=&range=day|week|month`; `GET /api/stats/online` → DevicesOnline(now-2*interval); `GET /api/stats/summary`; `GET/PUT /api/settings` (whitelist: `poll_interval_sec` 5–300; PUT применяет интервал поллера на лету); `POST /api/auth/password` {old,new} → VerifyPassword → AdminUpdatePassword + RefreshTokenRevokeAll; `GET /api/users/{id}/sub` → URL подписки + форматы (для UI copy).
  - `serve`: config.Load → store.Open → seed (если admin нет — лог «run admin init», HTTP всё равно стартует; дефолтный inbound + шаблоны + settings если пусто) → JWT key из settings `jwt_key` (generate 32B если нет) → Manager.Start → Poller.Run + RunBackup горутины → http.Server ListenAddr, graceful shutdown по SIGTERM (5 с).
  - `web/embed.go`: `//go:embed all:dist` → `var Dist embed.FS`.

- [ ] **Step 1: Падающие тесты**

`TestRouteAuthMatrix` — table (path × cookie → status): `/api/users` без cookie 401 / с валидным 200; `/api/auth/login` без cookie не 401 (400/401 от логики, не middleware); `/api/auth/2fa/enroll` без cookie 401; `/sub/xxx` без cookie не 401 (404 от логики); `/api/xray/status` без cookie 401.
`TestSPAFallback` (`GET /users` → 200 text/html содержит placeholder; `GET /api/nonexistent` → 404 JSON, не HTML).
`TestSettingsWhitelist` (PUT poll_interval_sec=60 ok; PUT unknown_key → 400; PUT 999 → 400).
`TestPasswordChangeRevokesRefresh`.
`TestXrayStatusEndpoint` (fake manager → JSON поля).

- [ ] **Step 2: FAIL**
- [ ] **Step 3: Реализовать**
- [ ] **Step 4: PASS** + `go vet ./...`
- [ ] **Step 5: Commit** `git checkout -b api-20261120-10 dev && git add -A && git commit -m "feat: router wiring, settings/xray/stats endpoints, serve, spa embed"`

---

### Task 11: Web scaffold + auth UI

**Branch:** `web-scaffold-20261120-11`

**Files:**
- Create: `web/package.json` (пины версий), `web/vite.config.ts`, `web/svelte.config.js`, `web/tsconfig.json`, `web/index.html`, `web/src/main.ts`, `web/src/App.svelte`, `web/src/app.css`, `web/src/lib/api.ts`, `web/src/lib/auth.svelte.ts`, `web/src/routes/Login.svelte`, `web/src/lib/components/` (shadcn-svelte init), `web/vitest.config.ts`
- Test: `web/src/lib/api.test.ts`, `web/src/lib/auth.test.ts`

**Interfaces:**
- Consumes: backend API (Task 10) — dev proxy `/api`,`/sub` → `127.0.0.1:3002`
- Produces:
  - `api.ts`: `apiFetch(path, opts)` — credentials include, JSON; на 401 → один POST /api/auth/refresh → retry один раз → иначе redirect на login. `login(username,password,totp?)`, `logout()`, `enroll2FA()`, `verify2FA(code)`, `disable2FA(code)`, `changePassword(old,new)`.
  - `auth.svelte.ts`: runes-store `{loggedIn, need2fa}` + `checkSession()` (GET /api/settings как probe).
  - App shell: sidebar (Дашборд, Юзеры, Inbounds, Routing, Шаблоны, Настройки, Конфиг), тёмная тема по умолчанию, svelte-spa-router, русский язык. Login: форма пароль → при `2fa_required` поле кода (TOTP или backup) → redirect /.
  - Версии пины: svelte 5.59.x, vite 8.3.x, @sveltejs/vite-plugin-svelte 7.3.x, tailwindcss 4.3.x, @tailwindcss/vite, shadcn-svelte 1.8.x, svelte-spa-router, qrcode, uplot, vitest. **Проверить актуальность при старте** (`npm view`), пинить exact.
- [ ] **Step 1: Падающие тесты** (vitest, mock fetch): `test('401 → refresh → retry once')`, `test('refresh fail → logout redirect')`, `test('login 2fa_required → need2fa state')`.
- [ ] **Step 2: FAIL** (`npx vitest run`)
- [ ] **Step 3: Реализовать** scaffold + api + auth + Login + shell. shadcn-svelte init (стиль default, dark, css variables).
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
- Produces: страницы. Users: таблица (username, трафик used/quota, онлайн-устройств, expiry, статус-бейдж disabled_reason), диалог create/edit (все поля: expiry datetime, quota_bytes, quota_period select none/день/неделя/месяц, device_limit + NAT-подсказка «один роутер = один IP»), delete confirm, кнопка сброса квоты, copy sub URL + 4 формата. Inbounds: список + форма (port, path, mode select, dest, serverNames, shortIds, fp) + raw JSON textarea + regenerate keys + enable toggle + delete; предупреждение «применение = рестарт xray ~1–2 с». Config: GET /api/config pretty-printed readonly + copy.
- [ ] **Step 1: Падающий тест** `Users.test.ts`: рендер таблицы с mock-данными (2 юзера, один disabled(quota) → бейдж «Квота»), кнопка create открывает диалог.
- [ ] **Step 2: FAIL**
- [ ] **Step 3: Реализовать**
- [ ] **Step 4: PASS** + build green.
- [ ] **Step 5: Commit** `git checkout -b web-pages-20261120-12 dev && git add -A && git commit -m "feat: web users, inbounds, config viewer pages"`

---

### Task 13: Web — Dashboard, Routing, Templates, Settings

**Branch:** `web-dashboard-20261120-13`

**Files:**
- Create: `web/src/routes/Dashboard.svelte`, `web/src/routes/Routing.svelte`, `web/src/routes/Templates.svelte`, `web/src/routes/Settings.svelte`, `web/src/lib/components/TrafficChart.svelte` (uPlot), `web/src/lib/traffic.ts` (трансформ данных)
- Test: `web/src/lib/traffic.test.ts`, `web/src/routes/Settings.test.ts`

**Interfaces:**
- Consumes: api.ts, /api/stats/*, /api/xray/*, /api/config/routing, /api/templates/*, /api/settings, /api/auth/2fa/*, /api/auth/password
- Produces: Dashboard — uPlot график трафика (переключатель день/неделя/месяц, select юзера/все), таблица онлайн-устройств (юзер, IP, last_seen), карточка xray (running/version/uptime/restarts + кнопки старт/стоп/рестарт с confirm), суммарный трафик. Routing — textarea JSON + кнопка «Проверить» (JSON.parse) + «Применить» (PUT) + предупреждение о рестарте. Templates — 3 таба (json/mihomo/clash) raw textarea + save + подсказка плейсхолдеров `{{UUID}}…`. Settings — смена пароля; 2FA: enroll → QR (`qrcode.toDataURL(otpauthURI)`) + секрет текстом + 10 backup-кодов (показать один раз, «сохрани») → verify кодом → включено; disable с кодом; интервал поллинга.
- [ ] **Step 1: Падающие тесты**: `traffic.test.ts` (buckets → uPlot series [timestamps, up, down], пустые данные → нули); `Settings.test.ts` (enroll mock → QR data URL сгенерирован, backup-коды отрендерены 10 шт).
- [ ] **Step 2: FAIL**
- [ ] **Step 3: Реализовать**
- [ ] **Step 4: PASS** + build green.
- [ ] **Step 5: Commit** `git checkout -b web-dashboard-20261120-13 dev && git add -A && git commit -m "feat: web dashboard, routing editor, templates, settings+2fa ui"`

---

### Task 14: Full build + embed + cross-compile + local smoke

**Branch:** `build-20261120-14`

**Files:**
- Modify: `Makefile`, `web/dist/index.html` (placeholder уже есть), `README.md` (build/run instructions, обезличено)
- Create: `scripts/smoke.sh` (локальный smoke, без реальных значений)
- Test: smoke-скрипт

**Interfaces:**
- Consumes: всё
- Produces: `make build` = npm ci → npm run build → go build -o bin/flypanel (SPA внутри); `make cross` = linux/amd64 бинарник; `scripts/smoke.sh`: поднять бинарник с dev .env (127.0.0.1:3002, XRAY_BIN — заглушка sleep-скрипт), создать админа через `flypanel admin init --password-stdin` (pipe), curl login → cookie → GET /api/users 200 → GET / → index.html (собранный SPA, не placeholder) → kill.
- [ ] **Step 1:** `make build` — green, бинарник содержит SPA (проверка: `curl /` отдаёт собранный index.html с хеш-ассетами).
- [ ] **Step 2:** `scripts/smoke.sh` — написать (admin создаётся через `flypanel admin init --password-stdin`, пароль из pipe), прогнать, PASS.
- [ ] **Step 3:** `make cross` — `file bin/flypanel-linux-amd64` → ELF 64-bit x86-64.
- [ ] **Step 4:** Grep-проверка обезличивания всего репо по **denylist из приватного runbook оператора** (файл вне репо, не коммитить): `grep -rInE -f <private-denylist-file> --exclude-dir=.git --exclude-dir=node_modules --exclude-dir=bin .` → **0 совпадений**. Сам denylist в репо не попадает.
- [ ] **Step 5: Commit** `git checkout -b build-20261120-14 dev && git add -A && git commit -m "feat: full build pipeline, embed spa, cross-compile, smoke"`

---

### Task 15: Deploy + E2E (OPERATOR-INTERACTIVE, выделенная VPS)

**Branch:** без ветки кода — ops-плейбук. **Выполняется в основной сессии, НЕ сабагентом. Каждый шаг — явное «да» пользователя.** Реальные значения — из приватного runbook оператора, не из репо. Результаты — в приватный журнал (вне репо). Целевой сервер — выделенная VPS (ADR-0008); старый сервер с Remnawave физически не затрагивается.

**Files:**
- Create: `docs/deploy-checklist.md` (обезличенный чеклист с плейсхолдерами)
- Серверные артефакты (вне репо): `/opt/flypanel/.env`, systemd unit, Caddyfile

**Interfaces:**
- Consumes: бинарник linux/amd64 (Task 14), spec §8
- Produces: работающий прод-стек + E2E-доказательства + журнал

- [ ] **Step 1: Порты + swap.** На VPS (read-only, разрешение): `ss -ltnp | grep -E ':80|:443|:8443|:3002|:10811'` → пусто (8080/8189 агента провайдера не трогать). Занято → СТОП, выбрать другие порты с оператором (обновить .env, не код). Swap: `swapon --show` пусто → создать 2 GB swapfile (`fallocate -l 2G /swapfile && chmod 600 /swapfile && mkswap /swapfile && swapon /swapfile` + запись в fstab) — отдельное «да».
- [ ] **Step 2: Файлы.** Создать /opt/flypanel/{bin,data,logs,backups}; загрузить бинарник; скачать xray **v26.11.9** официальный релиз linux-amd64 → bin/xray (checksum проверить); `.env` с реальными значениями (chmod 600): LISTEN_ADDR=`127.0.0.1:3002`, ENTRY_IP, EGRESS_IP= (пусто — один IP), PANEL_DOMAIN, XRAY_BIN, DATA_DIR.
- [ ] **Step 3: systemd.** Unit flypanel.service (User=flypanel, NoNewPrivileges, ProtectSystem=strict, ReadWritePaths=/opt/flypanel, Restart=on-failure); `flypanel admin init` (оператор вводит пароль); start; `journalctl -u flypanel --tail 30` — xray стартовал, poller работает.
- [ ] **Step 4: UFW.** allow 22/80/443/8443/tcp (мир). 3002/10811 — только loopback, правил наружу не нужно. Чужие правила (агент провайдера) не менять.
- [ ] **Step 5: DNS + Caddy (отдельное «да»). DNS:** оператор перенаправляет A-запись `<PANEL_DOMAIN>` на `<ENTRY_IP>` (сейчас указывает на старый сервер); дождаться распространения (DoH-проверка). **Caddy:** нативная установка (официальный репозиторий) → Caddyfile с нуля: блок `<PANEL_DOMAIN> { reverse_proxy 127.0.0.1:3002 }` → `caddy validate` → `systemctl reload caddy` (предупредить: reload рвёт established-соединения панели; других сайтов на этом Caddy нет, тихое окно не нужно) → верификация runtime :2019 + LE-сертификат + `curl https://<PANEL_DOMAIN>/` → SPA. Бэкап `cp -a Caddyfile Caddyfile.bak-YYYYMMDD` перед правками.
- [ ] **Step 6: E2E нового канала.** Создать тестового юзера через API/UI. На сервере: xray-клиент с конфигом из `/sub/<token>/json[0]` → `curl --socks5-hostname 127.0.0.1:<port> https://api.ipify.org` = **`<ENTRY_IP>`** (один IP; если задан EGRESS_IP — он). Все 4 формата подписки отдают валидный контент (base64 декодируется, json парсится, yaml валиден). Тестовый клиент виден в дашборде (IP + трафик > 0 после поллинга). Квота: поставить тестовому юзеру quota 1 МБ → после трафика disabled(quota) → сброс → enable.
- [ ] **Step 7: Старый стек.** Физически отдельный сервер — регрессия не требуется. Опционально (отдельное «да», read-only): старый канал жив, старая подписка отдаёт конфиги.
- [ ] **Step 8: Откат (готовность).** `systemctl stop flypanel`; закрыть 8443 в UFW; убрать Caddy-блок + reload; DNS вернуть при необходимости. Данные /opt/flypanel не трогаются.
- [ ] **Step 9: Журнал.** Запись в приватный журнал оператора (вне репо): что сделано, верификации, откат. В репо — только обезличенный `docs/deploy-checklist.md`.
- [ ] **Step 10: Commit** `git checkout -b deploy-20261120-15 dev && git add docs/deploy-checklist.md && git commit -m "docs: anonymized deploy checklist"`

---

## Self-review notes

- Spec coverage: §6.1→T4, §6.2→T2/T6/T9, §6.3→T3/T7, §6.4→T3/T7, §6.5→T7/T13, §6.6→T8, §6.7→T5/T9, §6.8→T1/T2/T9, §6.9→T4/T6/T7/T8/T10, §7→T11–T13, §8→T15, §9→branches+tests в каждом Task, §3→Global Constraints+T14 grep. Non-goals не реализуются.
- Решения билдера (spec §10), зафиксированные планом: миграции — embed SQL + schema_version (без зависимости); Reality keygen — Go crypto/ecdh (без subprocess); QR — npm `qrcode`; графики — uPlot; JSON-редактор — textarea+lint (без CodeMirror v1); роутер фронта — svelte-spa-router; периодный счётчик — агрегат из samples (без колонки-счётчика); курсор кумулятивных stats — в settings (переживает рестарт панели); retention samples 90д / devices 30д; username неизменяем; base64-шаблона нет (структурные ссылки); backup — VACUUM INTO горутиной (без cron); JWT key — settings, generate при первом старте; пароль админа ≥12 символов; `admin init --password-stdin` для автоматизации.
- Отклонения от эскиза spec (осознанные, на проверку оператору): (1) `/api/auth/2fa/*` **защищены JWT** (spec §6.9 пишет «кроме /api/auth/*», но публичный enroll позволил бы атакующему навязать свой 2FA и запереть админа — публичны только login/refresh/logout); (2) добавлен заголовок `Subscription-Userinfo` в /sub (стандарт де-факто для клиентов, spec не упоминает); (3) denylist обезличивания — файл вне репо, в репо не коммитится (сам список реальных значений = утечка).
- Amendment 20.11.2026 (ADR-0008, выделенная VPS): bind 127.0.0.1:3002 вместо docker-gateway; Caddy нативный с нуля (без тихого окна/inode-ловушек); EGRESS_IP пуст → sendThrough опущен (T3 `TestGenerateNoEgress`); swap 2 GB и DNS repoint добавлены в T15; регрессия старого стека не требуется (физически отдельный сервер); `.gitignore` создан заранее в корне репо.
