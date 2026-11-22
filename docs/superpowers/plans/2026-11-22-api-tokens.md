# API Tokens Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Admin can mint, list and revoke long-lived API tokens (`fp_…`, scopes `full`/`read`, expiry 30/90 days or never) that authenticate `/api/*` via `Authorization: Bearer`, and manage them from a restyled «Настройки» page.

**Architecture:** New `api_tokens` table (migration 0006) stores SHA-256 hashes only, exactly like `refresh_tokens`. `auth.Middleware` accepts either the `fp_access` cookie or a Bearer token, applies the read-scope denylist, and rejects Bearer on cookie-only paths. Three new handlers under `/api/tokens` are cookie-only mounted but listed by the same middleware.

**Tech Stack:** Go 1.x stdlib (`net/http` pattern routes, `crypto/rand`, `crypto/sha256`, SQLite via existing store), Svelte 5 runes, Vitest + @testing-library/svelte.

**Spec:** `docs/superpowers/specs/2026-11-22-api-tokens-design.md`

## Global Constraints

- Raw token format: `fp_` + base64url(32 random bytes); only SHA-256 hex is persisted.
- Scopes: `full`, `read`. Expiry choices: 30 days, 90 days, never (`expires_in_days` 30 / 90 / 0).
- Read scope: all non-GET/HEAD → 403; `/api/config`, `/api/config/routing`, any `/api/users*` → 403.
- Bearer is rejected (401) on every `/api/auth/*` path and on `POST`/`DELETE /api/tokens`.
- Cookie auth keeps working exactly as today, including on those paths.
- Changing the password revokes all API tokens as well as refresh tokens.
- Error bodies stay `{"error":"<code>"}` via existing `jsonError`; tokens are never logged.
- No new Go or npm dependencies. Icons are inline SVG.
- Existing Russian UI copy stays; new copy is Russian.
- Repo is public: fixtures use TEST-NET addresses and `example.com` only.

## Review Focus

- A token that expires exactly now, or whose row was revoked a second ago, must stop working on the next request (no caching of auth results).
- A read-scope token must not reach secrets indirectly: `/api/users/{id}/sub`, `/api/inbounds/{id}/keys`, `/api/config*` are covered, but a new route added later under `/api/users` must inherit the denial.
- Bearer on cookie-only paths must fail closed even when the same request carries a valid cookie-less body — no partial side effects.
- Creating a token with an empty/whitespace name or an unknown scope must not insert a row.
- After revoke, the one-time token block must disappear and the row must show as revoked without a full page reload.
- Clipboard copy must not throw when `navigator.clipboard` is unavailable (plain HTTP dev origin).

---

### Task 1: API token storage

**Files:**
- Create: `internal/store/migrations/0006_api_tokens.sql`
- Create: `internal/store/api_tokens.go`
- Test: `internal/store/api_tokens_test.go`
- Modify: `internal/store/store_test.go` (`allTables`, expected schema version)

**Interfaces:**
- Consumes: `fmtTime`, `parseTime`, `btoi`, `ErrNotFound`, `*Store` from existing store package.
- Produces:
  ```go
  type APIToken struct {
      ID        int64
      Name      string
      Scope     string
      ExpiresAt time.Time // zero = never expires
      Revoked   bool
      CreatedAt time.Time
  }
  func (s *Store) APITokenInsert(name, scope, hash string, expiresAt time.Time) (int64, error)
  func (s *Store) APITokenList() ([]APIToken, error)        // newest first
  func (s *Store) APITokenByHash(hash string) (APIToken, bool, error)
  func (s *Store) APITokenRevoke(id int64) error            // ErrNotFound when no row
  func (s *Store) APITokenRevokeAll() error
  ```

- [ ] **Step 1: Write the failing test** — `internal/store/api_tokens_test.go`:

```go
func TestAPITokenLifecycle(t *testing.T) {
    st := openTestStore(t) // existing helper style in the package
    exp := time.Now().Add(30 * 24 * time.Hour).UTC().Truncate(time.Second)
    id, err := st.APITokenInsert("ci", "read", "hash-1", exp)
    if err != nil { t.Fatalf("insert: %v", err) }
    never, err := st.APITokenInsert("ops", "full", "hash-2", time.Time{})
    if err != nil { t.Fatalf("insert never: %v", err) }

    list, err := st.APITokenList()
    if err != nil || len(list) != 2 { t.Fatalf("list: %v %d", err, len(list)) }
    if list[0].Name != "ops" || list[0].Scope != "full" { t.Fatalf("unexpected order: %+v", list[0]) }
    if !list[0].ExpiresAt.IsZero() { t.Fatalf("never token has expiry %v", list[0].ExpiresAt) }
    if !list[1].ExpiresAt.Equal(exp) { t.Fatalf("expiry %v != %v", list[1].ExpiresAt, exp) }

    got, ok, err := st.APITokenByHash("hash-1")
    if err != nil || !ok { t.Fatalf("by hash: %v %v", err, ok) }
    if got.ID != id || got.Revoked { t.Fatalf("unexpected row: %+v", got) }

    if _, ok, _ := st.APITokenByHash("nope"); ok { t.Fatal("unknown hash reported found") }

    if err := st.APITokenRevoke(id); err != nil { t.Fatalf("revoke: %v", err) }
    got, ok, _ = st.APITokenByHash("hash-1")
    if !ok || !got.Revoked { t.Fatalf("revoke not persisted: %+v", got) }

    if err := st.APITokenRevoke(id); err != store.ErrNotFound { t.Fatalf("second revoke: %v", err) }
    if err := st.APITokenRevokeAll(); err != nil { t.Fatalf("revoke all: %v", err) }
    got, _, _ = st.APITokenByHash("hash-2")
    if !got.Revoked { t.Fatal("revoke all missed row") }
    _ = never
}
```

Also update `store_test.go`: add `api_tokens` to `allTables` and change the expected schema version to `6`.

- [ ] **Step 2: Run to verify failure** — `go test ./internal/store/ -run TestAPITokenLifecycle -v` → FAIL (`s.APITokenInsert undefined`).

- [ ] **Step 3: Implement** — migration:

```sql
CREATE TABLE api_tokens (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  scope TEXT NOT NULL CHECK (scope IN ('full','read')),
  token_hash TEXT NOT NULL UNIQUE,
  expires_at TEXT NOT NULL DEFAULT '',
  revoked INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL
);
```

`api_tokens.go`: insert stores `fmtTime(expiresAt)` unless `expiresAt.IsZero()` then `''`; `APITokenList` `ORDER BY id DESC`; `APITokenByHash` maps `''` back to zero time; `APITokenRevoke`/`APITokenRevokeAll` use `UPDATE … SET revoked = 1` and return `ErrNotFound` on zero affected rows.

- [ ] **Step 4: Verify green** — `go test ./internal/store/ -v` → all pass, `TestOpenMigrates` reports version 6.

- [ ] **Step 5: Commit** — `git add internal/store && git commit -m "feat(store): api tokens table"`

---

### Task 2: Bearer authentication in middleware

**Files:**
- Modify: `internal/auth/jwt.go` (token minting)
- Modify: `internal/auth/middleware.go`
- Test: `internal/auth/middleware_test.go` (new), adjust `TestMiddleware` in `internal/auth/handlers_test.go` for the new signature

**Interfaces:**
- Consumes: `store.APIToken`, `store.Store` (auth already imports store), `hashToken`, `AccessCookie`.
- Produces:
  ```go
  func IssueAPIToken() (raw string, hash string, err error) // raw = "fp_" + base64url(32B)
  type APITokenStore interface {
      APITokenByHash(hash string) (store.APIToken, bool, error)
  }
  func Middleware(j *JWT, tokens APITokenStore) func(http.Handler) http.Handler
  ```

- [ ] **Step 1: Write failing tests** — `internal/auth/middleware_test.go` with a fake store (map of hash → `store.APIToken`):

```go
func TestMiddlewareAPIToken(t *testing.T) { /* cases below, each a subtest */ }
```

Cases (table with method, path, authorization header, expected status, expected next-called):
1. `GET /api/stats/summary` + valid full token → 200, next called.
2. `POST /api/inbounds` + valid read token → 403 `forbidden`, next not called.
3. `GET /api/config` + valid read token → 403.
4. `GET /api/config/routing` + valid read token → 403.
5. `GET /api/users/7/sub` + valid read token → 403.
6. `GET /api/stats/summary` + read token whose `ExpiresAt` is one second ago → 401.
7. `GET /api/stats/summary` + revoked token → 401.
8. `GET /api/stats/summary` + unknown hash → 401.
9. `POST /api/auth/login` + valid full token, no cookie → 401.
10. `POST /api/tokens` + valid full token, no cookie → 401.
11. `DELETE /api/tokens/3` + valid full token, no cookie → 401.
12. `GET /api/stats/summary` + valid cookie, no header → 200.
13. `GET /api/auth/2fa/enroll` + valid cookie → 200.

- [ ] **Step 2: Run to verify failure** — `go test ./internal/auth/ -run TestMiddleware -v` → compile error (signature mismatch).

- [ ] **Step 3: Implement** — `IssueAPIToken` mirrors `IssueRefresh` with the `fp_` prefix. Middleware order: cookie verify → if ok, `next`; else require `Bearer `, derive hash, look up, reject missing/revoked/expired (expiry check `!ExpiresAt.IsZero() && !ExpiresAt.After(time.Now())`); reject cookie-only paths (`strings.HasPrefix(r.URL.Path, "/api/auth/")`, and `r.Method` `POST`/`DELETE` with `r.URL.Path == "/api/tokens"`); reject read scope on non-GET/HEAD and on denylisted paths (`/api/config`, `/api/config/routing`, prefix `/api/users`); otherwise `next`. All rejections use existing `jsonError` with `401 unauthorized` / `403 forbidden`.

- [ ] **Step 4: Verify green** — `go test ./internal/auth/ -v` → all pass.

- [ ] **Step 5: Commit** — `git add internal/auth && git commit -m "feat(auth): accept bearer api tokens"`

---

### Task 3: Token endpoints

**Files:**
- Create: `internal/api/tokens.go`
- Modify: `internal/api/router.go` (routes + pass store to middleware)
- Test: `internal/api/tokens_test.go` (uses `newTestRouter`, `doReq`, `bodyJSON`, `createAdmin` from `router_test.go`)

**Interfaces:**
- Consumes: Task 1 store methods, Task 2 `auth.IssueAPIToken`, `auth.Middleware(j, st)`.
- Produces:
  - `GET /api/tokens` → 200 `{"tokens":[{"id":1,"name":"ci","scope":"read","expires_at":"2026-12-22T00:00:00Z"|"","created_at":"…"}]}`
  - `POST /api/tokens` body `{"name":"ci","scope":"read","expires_in_days":30}` → 201 `{"id":1,"name":"ci","scope":"read","expires_at":"…","token":"fp_…"}`
  - `DELETE /api/tokens/{id}` → 204, unknown id 404 `not_found`.

- [ ] **Step 1: Write failing tests** — cookie flow: create → list contains it → revoke → list shows it gone or revoked per chosen representation (list returns all rows with `"revoked":true`); duplicate revoke → 404; `{"name":"  ","scope":"read","expires_in_days":30}` → 400 `invalid_token_request`; `{"scope":"admin"}` → 400; `{"expires_in_days":7}` → 400; created token authenticates: `GET /api/stats/summary` with `Authorization: Bearer <token>` → 200 while cookie absent.

- [ ] **Step 2: Run to verify failure** — `go test ./internal/api/ -run TestAPITokens -v` → 404s / compile error.

- [ ] **Step 3: Implement** — handlers in `tokens.go`; router adds the three patterns to `p` and changes the mount to `auth.Middleware(d.JWT, d.Store)`.

- [ ] **Step 4: Verify green** — `go test ./internal/api/ -v` → all pass.

- [ ] **Step 5: Commit** — `git add internal/api && git commit -m "feat(api): token management endpoints"`

---

### Task 4: Password change revokes API tokens

**Files:**
- Modify: `internal/auth/handlers.go:351`
- Test: `internal/api/router_test.go` (`TestPasswordChangeRevokesRefresh`)

- [ ] **Step 1: Write failing assertion** — inside `TestPasswordChangeRevokesRefresh`, seed an API token via `st.APITokenInsert("ci", "full", "hash-x", time.Time{})`, perform the password change, then assert `st.APITokenByHash("hash-x")` returns `Revoked == true`.

- [ ] **Step 2: Run to verify failure** — `go test ./internal/api/ -run TestPasswordChangeRevokesRefresh -v` → FAIL (not revoked).

- [ ] **Step 3: Implement** — after `RefreshTokenRevokeAll()` add `h.Store.APITokenRevokeAll()`; on error respond 500 `revoke_failed`.

- [ ] **Step 4: Verify green** — `go test ./internal/api/ -run TestPasswordChangeRevokesRefresh -v` → PASS.

- [ ] **Step 5: Commit** — `git add internal/auth internal/api && git commit -m "fix(auth): revoke api tokens on password change"`

---

### Task 5: Settings page — token section and card restyle

**Files:**
- Modify: `web/src/routes/Settings.svelte`
- Test: `web/src/routes/Settings.test.ts`

**Interfaces:**
- Consumes: `apiFetch`, `Button`, `Input`, `Label`; endpoints from Task 3.
- Produces: section markup only; no exports.

- [ ] **Step 1: Write failing tests** — add to `Settings.test.ts` (fetch stub answering `/api/settings`, `/api/tokens`):
  - renders a row `ci` with scope text «Только чтение» and expiry «Никогда» from `{"tokens":[{"id":1,"name":"ci","scope":"read","expires_at":"","created_at":"2026-11-22T10:00:00Z"}]}`;
  - typing name + clicking «Создать» POSTs `{"name":"ci","scope":"read","expires_in_days":30}` and shows the returned `fp_` string once;
  - clicking «Отозвать» on a row sends `DELETE /api/tokens/1`.

- [ ] **Step 2: Run to verify failure** — `cd web && npx vitest run src/routes/Settings.test.ts` → FAIL.

- [ ] **Step 3: Implement** — on mount `loadTokens()` alongside settings. Section order: «Токены API», «Двухфакторная аутентификация», «Смена пароля», «Интервал поллинга». Each card: header row with 36px icon square (inline SVG, `rounded-lg bg-sky-500/15 text-sky-300`) + title `text-base font-medium` + subtitle `text-sm text-muted-foreground`. Token card: table `НАЗВАНИЕ / ПРАВА / ИСТЕКАЕТ / —` with revoked rows dimmed and label «Отозван»; create form (name input, two `<select>` for scope and expiry); after create a bordered block with the raw token in `font-mono`, button «Копировать» (guard `navigator.clipboard?.writeText`) and note «Токен показывается один раз». Use a plain «Отозвать» text button per row instead of a kebab menu — the repo has no dropdown primitive.

- [ ] **Step 4: Verify green** — `cd web && npx vitest run src/routes/Settings.test.ts` → PASS, then `npx vitest run` → whole suite green, `npm run check` → 0 errors.

- [ ] **Step 5: Commit** — `git add web/src && git commit -m "feat(web): api tokens section in settings"` (include rebuilt `web/dist/index.html` if `make build` was run).

---

## Final verification

- [ ] `make test` → all Go packages pass.
- [ ] `cd web && npx vitest run` → all tests pass.
- [ ] `make cross` → binary builds.
- [ ] Manual smoke against the local panel (`http://127.0.0.1:3002`, admin `FlyPanel-2026`): create `full` token, `curl -H "Authorization: Bearer fp_…" /api/stats/summary` → 200; same on `/api/config` with a `read` token → 403; revoke → 401.