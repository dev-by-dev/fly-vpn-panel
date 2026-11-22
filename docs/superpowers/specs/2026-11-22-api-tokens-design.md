# API tokens and the new Settings layout — design

## Why

The panel is controlled from a browser only: the whole `/api/*` is closed by the session
JWT cookie `fp_access` (15 minutes + refresh). Automation (scripts,
monitoring, CI) is forced to log in with a password and keep a cookie session.
We need a way to give an external client a long-lived key with limited
rights and the ability to revoke it — and to see it in “Settings” the way the
mockup shows: an “API tokens” card with a `Name / Scope / Expires` table,
a refresh/“+” toolbar and a “…” menu.

At the same time the three existing Settings cards (password, polling interval,
2FA) are brought to the same look: an icon in a rounded square, a title
+ subtitle, uniform spacing. The behaviour of these three blocks does not change.

## Goals

- Create a token in the UI, see it once, copy it.
- Token list: name, scope, expiry; revoke via “…”.
- Token acceptance in the API: `Authorization: Bearer fp_…` on any `/api/*`.
- Two scopes: `full` (all methods) and `read` (GET only).
- Lifetime: 30 days, 90 days, unlimited.
- Browser cookie authentication works as before, unchanged.

## Non-goals (in this branch)

- `last_used_at`, usage counters, per-token limits.
- Tokens scoped to specific sections, tokens per VPN user.
- Multi-language UI (ru/en/de) — a separate branch after this one.
- Changing the behaviour of password change, polling, 2FA.

## Anti-goals (we count these as failure)

- Storing the token in plaintext in the DB or in the logs.
- Any way to show the token again after creation.
- A token whose access cannot be revoked.
- Regression: the browser panel stops working because of the new middleware.

## Options considered

**A. A separate tokens table, hash + scope + expiry, Bearer in the middleware —
chosen.** It matches the existing refresh-token pattern
(32 random bytes, sha256 hex in the DB), revocation and expiry come for free,
the scope adds ~15 lines.

**B. Hand the client the JWT secret itself / a long-lived JWT.** Zero new
code, but revocation is impossible, the lifetime is not selectable, one key for everything.
Rejected.

**C. OAuth2 client credentials.** Full-featured, but requires a consent screen,
`client_secret` storage, per-endpoint scopes — an order of magnitude
more code for a single scenario. Rejected.

**D. Tokens without scopes (full only).** One select and one check simpler,
but the mockup has a “Scope” column, and a read-only token is
a real monitoring need. Two scopes kept.

## How it works

### Storage

Migration `internal/store/migrations/0006_api_tokens.sql`:

```sql
CREATE TABLE IF NOT EXISTS api_tokens (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  name       TEXT    NOT NULL,
  scope      TEXT    NOT NULL CHECK (scope IN ('full','read')),
  token_hash TEXT    NOT NULL UNIQUE,
  expires_at TEXT    NOT NULL DEFAULT '',   -- '' = never expires
  revoked    INTEGER NOT NULL DEFAULT 0,
  created_at TEXT    NOT NULL
);
```

Time is text in the same format as in the other tables (`fmtTime`).
Access layer — `internal/store/api_tokens.go`: `APITokenInsert`,
`APITokenList`, `APITokenByHash` (returns scope/expiry/revoked),
`APITokenRevoke`.

### Token

`auth.IssueAPIToken() (raw, hash string, err error)`:
32 random bytes → `base64.RawURLEncoding`, the `fp_` prefix on `raw`,
`hash` = sha256 hex (the same `hashToken` as for refresh). Length of
`raw` = 46 characters. The format is fixed: `fp_` + base64url.

### Verification

`auth.Middleware(j *JWT, tokens APITokenStore)` — the signature grows,
`APITokenStore` is the interface `{ APITokenByHash(hash string) (APITokenInfo, bool, error) }`
(`APITokenInfo{Scope string; ExpiresAt time.Time; Revoked bool}`), so that
tests can substitute a fake and `store` is not pulled into `auth`.

Order: cookie `fp_access` → if valid, let it through (as now);
otherwise the `Authorization: Bearer fp_…` header → hash → lookup in the DB →
reject on `revoked`, on an expired `expires_at`, on an unknown hash.
Otherwise 401 `{"error":"unauthorized"}` (the text does not change).

On success with a `read`-scoped token: if the method is not `GET` (and not
`HEAD`) → 403 `{"error":"forbidden"}`. The check lives in the middleware,
so it covers every mutating endpoint at once; handlers are untouched.

### Endpoints

- `GET /api/tokens` → `[{id,name,scope,expires_at,revoked,created_at}]`,
  without hashes. Sorting: newest first.
- `POST /api/tokens` `{name,scope,expires_in_days}` → 201
  `{id,name,scope,expires_at,token}`; `token` — the only time.
  Validation: `name` after trim 1–64 characters, `scope` ∈ {full,read},
  `expires_in_days` ∈ {0,30,90} (0 = never expires). Error → 400
  `{"error":"invalid_token_request"}`.
- `DELETE /api/tokens/{id}` → 204; an unknown or already revoked id → 404
  `{"error":"not_found"}`. Repeated revocation is not idempotent: 404, so the UI
  can tell “just revoked” from “no such token”.

Handlers — a new file `internal/api/tokens.go`, the `Handlers` type already carries
`Store`.

### UI (Settings)

Page `web/src/routes/Settings.svelte`, styled per the mockup: card
`rounded-2xl border bg-card`, header — a 40×40 icon in `rounded-xl
bg-…/10` + `h2` + a `text-sm text-muted-foreground` subtitle, a divider,
the body, and where there are actions — a bottom bar with an icon button
“Refresh” on the left and a primary “+” on the right.

Sections in order: API tokens → Two-factor authentication → Password
change → Polling interval.

API tokens:
- table: columns `Name`, `Scope`, `Expires`, the last column is the
  “…” button; a green dot for an active token, grey/red for an expired and a
  revoked one; scope `full` → “Full access”, `read` → “Read only”;
  an empty expiry → “Never expires”, otherwise `dd.MM.yyyy`;
- “+” expands an inline form: name, scope (select), lifetime (select
  30/90/never) → “Create”; after success the form is replaced by a
  “The token is shown once — save it now.” block: a `code` with the token + “Copy”;
- “…” → “Revoke” (no `confirm()` dialog: revocation marks the row,
  no data is lost);
- “Refresh” re-fetches the list;
- an empty list → “No tokens yet.”.

Messages: `Failed to load (${status})`, `Network unavailable`,
`Token created`, `Token revoked`, `Error (${status})`.

## What was decided (and why)

- A Bearer header, not a query parameter: the token does not end up in proxy logs.
- sha256 hash, not bcrypt: the token is high-entropy, brute force is not a concern,
  comparison is cheaper — the same argument as for refresh tokens.
- The scope is checked in the middleware, not in every handler: one place,
  impossible to forget in a new endpoint.
- A revoked row stays in the table: history is visible, revocation
  is idempotent.
- The `fp_` prefix: the token is recognizable in logs/configs, leaks are easier to find.

## Fixes after spec review

Three holes found in review are closed in the same branch:

1. **`read` does not grant access to secrets.** `GET /api/config` returns the
   whole `xray.json` (private keys), `GET /api/users` — `uuid` and
   `sub_token` of every user, so a monitoring token turned into
   credential theft. For `read` tokens an extra deny list of paths: `/api/config*`,
   `/api/inbounds*` (except `/api/inbounds/metrics`), `/api/users*`,
   `/api/monitoring*` → 403 `forbidden`. Everything else (`stats/*`, `xray/status`,
   `system`, `devices`, inbound metrics) is readable.
2. **A token cannot manage authentication and tokens.** On
   `/api/auth/*` (including password change and 2FA disable) and on the whole
   `/api/tokens` (including `GET`) Bearer is not accepted at all: cookie session
   only, otherwise 401. Otherwise a leaked token could disable 2FA,
   change the password or issue itself a new token — which breaks the very
   idea of revocation.
3. **Password change revokes API tokens too.** Changing the password hash and revoking
   refresh and API tokens happen in one transaction
   (`AdminUpdatePasswordAndRevokeTokens`), so the message “all sessions
   revoked” is true even if something fails halfway.

## Left to the implementer

- The exact names of the store layer's Go functions and the `expires_at` format — following the
  neighbouring files.
- Table markup and icon classes — following the existing cards,
  so as not to introduce a second visual language.
- Whether separate styles are needed for `select` (the project has no UI primitive
  for select) — a native `<select>` with the `Input` classes.

## Verification

- Go: store tests (insert/list/by-hash/revoke, expired and revoked,
  `APITokenRevokeAll`), handler tests (create 201 + token shown once,
  list without hashes, delete 204/404, validation 400), middleware tests
  (cookie, a valid Bearer, read + POST → 403, read +
  `/api/config` and `/api/users` → 403, Bearer on `/api/auth/*` and
  `POST /api/tokens` → 401, revoked → 401, expired → 401),
  a `ChangePassword` test — after a password change both the tokens and refresh
  tokens are revoked.
- Web: `Settings.test.ts` — the list renders, “+” creates a token and
  shows it, “Revoke” sends the DELETE and updates the row, a load error
  is shown and does not break the other sections.
- E2E manually: create a `full` token in the panel → `curl -H "Authorization: Bearer
  fp_…" https://panel.example.org/api/users` → 200; with a `read` token the same
  request → 403, and `POST /api/users` → 403; after “Revoke” → 401.
- Regression: the browser panel works after login (the cookie path is not
  affected), `make test`, `cd web && npx vitest run`.
