# fly-vpn-panel — design doc (v1)

Date: 2026-11-20. Status: approved by the user on 2026-11-20.
Process: SDD. Branches: `<feature>-20261120-NN` from `dev`. Repo: **public** GitHub, all artifacts anonymized (§3).
Architecture decisions: `docs/adr/` (ADR-0001…0008, accepted).

## 1. Intent and why

Our own web panel for a VPN server. The pains of the current Remnawave (confirmed by the user):

1. **Uninformative**: after creating a profile it is unclear which devices are connected and how much traffic they consume.
2. **The template sanitizer** cuts routing rules, the subscription generator is broken (CLASH is always empty, xPadding fields are not emitted).
3. **Unnecessary complexity**: panel + node + postgres + valkey — four entities instead of one.
4. We want **full control over the xray config without an indirection layer**, like in 3x-ui.

The new panel is the **future replacement for Remnawave** (not a parallel toy tool). Remnawave keeps running as is on the old server; the new panel is deployed on a **dedicated VPS** (ADR-0008) — the existing stack is not physically touched.

The main v1 feature is visibility: who is connected (devices) + how much traffic (per user).

## 2. Goals / Non-goals / Anti-goals

**Goals v1 (confirmed by the user: “v1 yes”):**
- Auth: one admin, password + JWT + optional TOTP/2FA.
- xray lifecycle: start/stop/restart, config validation BEFORE applying.
- Inbound CRUD: VLESS + XHTTP + Reality.
- Users CRUD + expiry + traffic quota (reset period + auto-disable/enable) + device limit by distinct online IP.
- Traffic per user + online devices (hybrid, see §6.7).
- Subscriptions: 4 formats (base64 / json / mihomo / clash).
- Routing via a JSON editor of the full config (no sanitizer).
- Dashboard: traffic day/week/month, online, xray state.
- Deploy on a dedicated VPS behind a `<PANEL_DOMAIN>` subdomain with our own Caddy.

**Non-goals v1 (the user confirmed “later”):** singbox/stash formats · visual rule builder + preset library + a doc on “how to create rules” · multi-node · per-inbound membership (squads) · migrating users from Remnawave · Telegram notifications · API tokens for scripts.

**Anti-goals (failure even if it technically works):**
- Touching the running stack on the old server during deploy (the dedicated VPS rules that out by construction; any action on the old server needs a separate explicit “yes”).
- A sanitizer / silent corruption of user rules and configs.
- Opaque generation: the user must see the final xray config and the final subscription.
- A relapse into container zoo complexity: deploy = one binary + xray.
- Leaking real addresses/names/paths into the public repo (§3).

## 3. Constraints

**Public repo — anonymization (user directive 20.11.2026):**
- No real IPs, domains, server/provider/user names, secret paths or client-network details in any repo artifact (docs, code, comments, fixtures, tests).
- Placeholders: `<ENTRY_IP>` (the VPS public IP, ingress=egress), `<PANEL_DOMAIN>` (UI + subscription domain), `<EGRESS_IP>` (optional separate egress IP; empty → `sendThrough` is omitted; groundwork for a future “ingress≠egress”).
- Real values live ONLY on the server (`/opt/flypanel/.env`, chmod 600, gitignored) and in the operator's private runbook (outside the repo).
- The git history must be clean BEFORE the first push (rewrite the draft spec commit).

**Dedicated VPS (ADR-0008, recon 20.11.2026):**
- Ubuntu 24.04 LTS, 1 vCPU / 1 GB RAM / 10 GB disk, one public IP (ingress=egress). No swap → create a 2 GB swapfile at deploy. Docker is not needed: Caddy is native, the panel + xray run under systemd.
- Access only through the operator's SSH tooling, **explicit permission for every action**; do not change the SSH port. Do NOT TOUCH the provider agent (8080 / 127.0.0.1:8189).
- The old server with the running Remnawave stack is physically a separate machine; deploying does not affect it by construction. The only permitted contact is read-only reading of production templates (§6.6).
- Ports 80/443/8443/3002/10811 are free (verified by recon). The panel binds **127.0.0.1:3002**, the xray API **127.0.0.1:10811**, the VPN ingress `<ENTRY_IP>:8443`.
- Freedom outbound without `sendThrough` (one IP). `<EGRESS_IP>` is empty → the generator omits the field; a second IP can be added later without code changes.
- DNS: the `<PANEL_DOMAIN>` A record currently points at the old server — the operator will repoint it to `<ENTRY_IP>` at deploy. Caddy obtains the LE certificate itself (HTTP-01, port 80).
- Clients: modern mihomo cores ≥1.19.22 (FlClash, Clash Verge) and v2rayNG support VLESS+XHTTP+Reality. Classic Clash (core <1.19.22) is not supported.
- Do not print secrets; record the work results in the operator's private log (outside the repo).

## 4. Approaches (what we considered and why)

| Decision | Chosen | Alternatives and why not |
|---|---|---|
| Topology | **A** (user's choice, ADR-0001): Reality for the VPN on its own port 8443, xray terminates TLS itself; the UI behind our own Caddy on a subdomain | B: everything behind one Caddy — ties the VPN to reload risks, an h2c shim, not “full control”. C: a second Caddy on 8443 + LE DNS-01 — an extra entity and certificate fiddling, no gain |
| Placement | **Dedicated VPS** (user's choice 20.11.2026, ADR-0008): the new stack alone on the server, the old one untouched | Coexisting on the old server — risk for the prod stack, quiet reload windows, shared ports, docker-gateway bind |
| Backend + DB | **Go + SQLite, one binary** (Go is the user's requirement; SQLite confirmed, ADR-0002): WAL + `_busy_timeout`, backup = a file copy | Postgres — +container/backups/RAM/maintenance, zero gain until thousands of users/multi-node. Node/Rust/Python — heavier/slower/no native gRPC advantage. Precedent: 3x-ui = Go+SQLite+xray |
| xray ownership | **A child process of the panel** (3x-ui style, ADR-0003): the panel owns the whole lifecycle | A separate systemd unit — two owners, desync; a node agent — the indirection we are leaving behind |
| Frontend | **Svelte 5 + Vite + Tailwind v4 + shadcn-svelte**, the SPA is embedded into the Go binary via `embed.FS` (my choice, delegated; ADR-0004): deploy = one file, light bundle | React/Next — heavier, SSR not needed; server templates — not “pretty/modern” |
| Visibility | **Hybrid via the xray stats API** (my choice, ADR-0005): traffic per user (QueryStats) + online devices IP/last-seen (**GetStatsOnlineIpList / GetAllOnlineUsers** — xray returns online IPs per user itself; parsing the access log is NOT needed) | Per-device traffic — xray stats are per uuid only, cannot be done reliably; access-log parsing — harder and unnecessary given the API. NAT caveat: one IP for the whole router, “device” = distinct source IP |
| Quota | **Reset period + auto-revert** (user's choice, ADR-0006): up+down, period per user (no reset/day/week/month) | Lifetime + manual reset — simpler, but does not match the Remnawave habit |

## 5. Architecture and topology

```
client → vless://<ENTRY_IP>:8443 (Reality, sni=dest site, XHTTP stream-up)
       → xray (child process of flypanel) → freedom (sendThrough <EGRESS_IP> if set) → internet

browser → https://<PANEL_DOMAIN> → Caddy :443 (our own, native)
       → reverse_proxy 127.0.0.1:3002 → flypanel (Go, SPA embed)
       → SQLite /opt/flypanel/data/panel.db
       → gRPC 127.0.0.1:10811 (dokodemo-door API inbound in xray)
       → subscriptions: https://<PANEL_DOMAIN>/sub/<token>[/base64|/json|/mihomo|/clash]
```

- **flypanel**: one Go binary, systemd unit `flypanel.service`, listens on **127.0.0.1:3002** (loopback: native Caddy can reach it, it does not stick out). The SPA is embedded (`embed.FS`).
- **xray**: binary from the official release **v26.11.9** (pinned at deploy) in `/opt/flypanel/bin/xray`, started by the panel as a child process. Config `/opt/flypanel/data/xray.json` — generated by the panel from the DB (the DB = source of truth).
- **xray API inbound**: dokodemo-door `127.0.0.1:10811` (panel on the host → loopback works), tag `api`. StatsService + HandlerService + routing service.
- Layout `/opt/flypanel/`: `flypanel`, `bin/xray`, `data/panel.db`, `data/xray.json`, `logs/`, `.env` (real values, chmod 600, gitignored).
- UFW at deploy: allow 22/80/443/8443/tcp (world). 3002/10811 — loopback only, no outward rules needed. Create 2 GB swap.
- Caddy: **native install** (the official static binary from GitHub Releases + our own systemd unit, see the installer spec), Caddyfile from scratch: block `<PANEL_DOMAIN> { reverse_proxy 127.0.0.1:3002 }`. Back up with `cp -a Caddyfile Caddyfile.bak-YYYYMMDD` before edits, `caddy validate` → `systemctl reload caddy`, an explicit user “yes”, runtime verification :2019 + LE. No quiet window needed (on this Caddy only our site); a reload drops established panel connections — warn about it.

## 6. Backend components

### 6.1 Auth
- One admin. Created by a **CLI on the server** `flypanel admin init` (interactively: password). NO public registration (anti-goal: someone registers first).
- Password: argon2id. JWT: access 15 min + refresh 7 days, httpOnly cookies, SameSite=Strict; refresh tokens in the DB (revoked on logout/password change).
- **2FA TOTP (RFC 6238, pquerna/otp v1.5.0)**: optional to enable, mandatory at login once enabled. Enrollment: secret + `otpauth://` URI → **the frontend renders the QR** (npm lib, no Go dependency for QR). 10 backup codes (hashes, single-use). Disabling only with a current TOTP/backup code.
- Login rate limit (e.g. 5 attempts/min per IP).

### 6.2 Users
- Fields: username, uuid (VLESS id), sub_token (short random), expiry, quota_bytes (0 = unlimited), **quota_period (none|day|week|month)**, quota_reset_at, device_limit (0 = unlimited, **default 0**), disabled, disabled_reason (quota|device_limit|expired|manual), created.
- **Quota (ADR-0006)**: counted as **up+down combined**. The period counter = the sum of deltas from traffic_samples since the period start. Exceeding → hot removal from the inbound + disabled_reason=quota. A new period starts → the counter resets, the user is **auto-enabled** (if disabled by quota). The admin can reset/raise the limit manually → auto-enable. Period none = lifetime limit (re-enable only manually).
- **device_limit**: hard, by **concurrent** distinct online IPs (GetStatsOnlineIpList). Exceeding → hot disable (disabled_reason=device_limit); **auto-revert** once online IPs are ≤ the limit again (next poll). NAT caveat in the UI: one router = one IP; phone+home = two. Default unlimited. Known limitation: a concurrent limit does not prevent rotating devices one at a time — deliberate (a cumulative window comes later if needed).
- **expiry**: expiry → hot disable (disabled_reason=expired), checked both on the poll and on subscription requests. The admin extends → auto-enable on the next poll.
- **Membership v1**: all active users belong to **all enabled inbounds** (no join table). Per-inbound squads are a v1 non-goal. The subscription returns links for all enabled inbounds.
- xray stats are keyed by email = username.

### 6.3 Inbound / protocol
- v1: VLESS + XHTTP + Reality. A structured form for the main parameters: port (default 8443), path (default `/xhttp`), mode (default stream-up), Reality dest (default **`www.microsoft.com:443`** — TLS1.3+H2; configurable), serverNames, privateKey/publicKey (generated by the panel), shortIds, fingerprint (default chrome).
- **Raw JSON** for the remaining inbound parameters (3x-ui style) — full control, no sanitizer.
- The data model supports N inbounds; the UI manages the list.
- The resulting xray config is always visible in the UI (the “Config” tab).

### 6.4 xray lifecycle and config application
- **Structural changes** (routing, inbound parameters, a new inbound): build the JSON from the DB → **validate with `xray run -test -c`** → only on success write the file → restart the child process (~1–2 s downtime, the UI warns). On validation failure the old config is untouched and the error shows in the UI.
- **Users/quotas/limits/expiry**: hot via gRPC `HandlerService.AlterInbound` (AddUserOperation/RemoveUserOperation) — no restart, connections stay alive.
- xray crash → auto-restart with backoff, status/alert in the UI. Start/stop/restart via buttons.
- Policy in the config: level 0 `statsUserUplink/statsUserDownlink/statsUserOnline = true` (otherwise online IPs are not collected), users at level 0. Outbounds: `freedom` (sendThrough `<EGRESS_IP>` **if set**, otherwise omitted), `block`. Routing v1: empty rules by default (the user adds them via the editor).

### 6.5 Routing
- v1: a **JSON editor** for the routing section of the full config in the UI. No sanitizer — what you type is what gets applied (after validation).
- Presets/visual builder/a doc on “how to create rules” — later (v1 non-goal).

### 6.6 Subscriptions
- URL: `https://<PANEL_DOMAIN>/sub/<token>`; suffixes `/base64` `/json` `/mihomo` `/clash`. Default URL: UA fallback (v2rayNG → base64; FlClash / clash-verge lowercase → mihomo; anything else → base64).
- Formats:
  - **base64**: a list of `vless://` links (Reality+XHTTP parameters: type=xhttp, path, mode, security=reality, pbk, sid, sni, fp, encryption=none).
  - **json**: an array of client xray configs (one inbound → one config; routing from the template).
  - **mihomo**: yaml (proxies + proxy-groups + rules).
  - **clash**: yaml, mihomo-compatible (all target Clash clients are on a core ≥1.19.22; classic Clash is unsupported — non-goal).
- **Per-format templates**: editable raw text in the UI with placeholders, stored in the DB, **without a sanitizer**. On install the default templates **mirror the current production templates** of the running panel (RU-direct rules, the adblock rule-provider anti-ad.net + 10 Google/YouTube domains, sniffer, `ipv6: false`, Telegram IP-CIDR → proxy no-resolve) — so the replacement does not lose settings. Source: the operator **permitted read-only reading** of the current templates (files/DB of the running panel) during implementation; the exact paths are in the private runbook, not the repo. The mirror is verified by a diff. This is the starting text; the user edits freely.
- Public access: only `/sub/*` without JWT; the token is the user's secret.

### 6.7 Visibility and statistics (the main feature)
- A gRPC poller every 30 s (configurable):
  - `QueryStats` per-user uplink/downlink (pattern `user>>><email>>>traffic>>>uplink`, reset=false; 3 arrows — verified against the v26.11.9 sources) → deltas into SQLite.
  - `GetAllOnlineUsers` + `GetStatsOnlineIpList` per user → online IPs + last-seen. **Semantics confirmed against the v26.11.9 sources (T5):** GetStatsOnlineIpList → per-user IPs (map `user>>>email>>>online`), GetAllOnlineUsers → online emails. No access-log fallback needed.
- SQLite: `traffic_samples(user, ts, up_delta, down_delta)` → day/week/month aggregates + the period counter for the quota; `devices(user, ip, first_seen, last_seen)`.
- Enforcement on the poll: quota / device_limit / expiry (§6.2) → hot RemoveUser + disabled_reason; auto-revert per §6.2.
- xray counters reset on restart — deltas are recorded often (30 s polling), loss ≤ the interval.
- Dashboard: traffic charts per user (day/week/month), the online device list (user, IP, last-seen), xray state (running/version/uptime/restarts), total traffic.

### 6.8 DB (SQLite, modernc.org/sqlite v1.62.0, pure Go)
- DSN: `_journal_mode=WAL&_busy_timeout=5000&_synchronous=FULL` (the `_synchronous` key — modernc only recognizes that one; a bare `synchronous=` is ignored). One writer.
- Tables (sketch): `admin`, `users`, `inbounds`, `settings` (key/value: templates, Reality keys, polling interval), `traffic_samples`, `devices`, `refresh_tokens`.
- Backup: a daily cron `sqlite3 panel.db ".backup …"` (or the Go equivalent), 7 copies retained. Restore = copy the file back.
- Migrations: schema versioning (tooling at the builder's discretion).

### 6.9 API (sketch)
`/api/auth/login|refresh|logout|2fa/enroll|2fa/verify|2fa/disable` · `/api/users` CRUD · `/api/inbounds` CRUD · `/api/config` (GET/PUT raw routing + viewing the resulting config) · `/api/xray/status|start|stop|restart` · `/api/stats/*` · `/api/templates` CRUD · `/api/settings`. JWT middleware on everything except `/api/auth/*` and `/sub/*`. HTTP router: stdlib `net/http` (Go ≥1.22 patterns) — enough, no framework needed (my choice; the builder may justify otherwise).

## 7. Frontend

- **Svelte 5.59 (runes) + Vite 8.3.8 (Rolldown) + @sveltejs/vite-plugin-svelte 7.3.1 + Tailwind CSS 4.3.5 (@tailwindcss/vite) + shadcn-svelte 1.8.1** (versions verified by web research 20.11.2026, compatible; shadcn-svelte ≥1.0 = Svelte 5 + TW4 only; Node ≥20.19/22.12).
- SPA, build → `embed.FS` of the Go binary. Dark theme in the Remnawave spirit. **UI languages — ru/en/de** (originally Russian only; localization is a separate spec).
- Pages: Login (+2FA) · Dashboard · Users · Inbounds/Protocol · Routing (JSON editor) · Templates (subscriptions) · Settings (2FA, password, xray, intervals) · Config viewer (the resulting xray.json).
- QR for TOTP — an npm lib on the frontend (the builder chooses). Charts — a light lib (the builder chooses, no heavy dashboard frameworks).

## 8. Deploy and verification (implementation phase, not now)

Real values (IPs, domains, ports, paths) are substituted from the operator's private runbook on the server — not from the repo.

The target server is a **dedicated VPS** (ADR-0008); the old server with Remnawave is physically untouched.

1. Build on the operator's machine: Go cross-compile linux/amd64 + embedded SPA. Binary to the server through the operator's SSH tooling.
2. VPS preparation (each step — an explicit “yes”): swap 2 GB; check ports 80/443/8443/3002/10811; create /opt/flypanel; download xray v26.11.9 (official release + checksum); systemd unit; `flypanel admin init`; UFW allow 22/80/443/8443.
3. DNS: the operator repoints the `<PANEL_DOMAIN>` A record to `<ENTRY_IP>`. Caddy: native install → the `<PANEL_DOMAIN>` block → validate + reload → verify :2019 + the LE certificate.
4. **E2E of the new channel**: an xray client on the server through 8443 → `curl --socks5 … api.ipify.org` = **`<ENTRY_IP>`** (one IP; or `<EGRESS_IP>` if set). Subscriptions: all 4 formats return valid content. Online stats: the test client is visible on the dashboard (IP + traffic).
5. The old stack is physically separate — no regression there is required. Optionally: a read-only check of the old channel (a separate explicit “yes”).
6. Rollback: `systemctl stop flypanel`, close 8443 in UFW, remove the Caddy block + reload, restore DNS if needed. The /opt/flypanel data is not touched.
7. Record the result in the operator's private log (outside the repo).

## 9. Process (SDD)

- This doc → user approval → the writing-plans skill → the implementation plan.
- Architecture decisions → `docs/adr/` (Nygard format; proposed → accepted after spec approval; new decisions — new ADRs).
- Branches `<feature>-20261120-NN` from `dev` (e.g. `auth-20261120-01`, `xray-lifecycle-20261120-02`); merge into `dev`, `main` at milestones.
- Tests: Go unit tests for subscription generators, quota/device/expiry enforcement, auth/TOTP; the §8 E2E playbook is manual on the server.
- Each feature = a separate plan step with verification.
- Anonymization (§3) is checked before every push.

## 10. What is left to the builder

- The Go package layout, the migrations tool, the Reality key generation method (xray x25519 subprocess vs Go crypto), the npm QR/chart libs, polling tuning, the exact syntax of the Caddy block and systemd hardening, the details of the test strategy.
- `.gitignore` is created at the repo root (secrets, DB, binaries, logs, backups, build artifacts) — extend as needed.
- Verifying the GetStatsOnlineIpList/GetAllOnlineUsers semantics — **done (T5, §6.7)**: per-user IPs confirmed against the v26.11.9 sources.
- npm versions may have drifted by install time — pin at the start of implementation.
- The mechanics of the quota period counter (a column vs an aggregate from samples) — a schema detail.

## 11. User decisions (recorded in the dialogue)

- Go backend · SQLite · SDD · branches `<feature>-20261120-NN` · frontend “light, concise, beautiful, modern” (the choice was delegated to me) · a “light and fast” DB (the choice was delegated, SQLite confirmed).
- Subscriptions are needed; the panel is the future replacement for Remnawave; RU-direct/adblock rules are configurable, a doc on “how to create rules” comes later.
- One admin, password + JWT + **OTP/2FA**.
- Topology **A**.
- Visibility: **hybrid** (traffic per user + devices IP/last-seen).
- v1 scope confirmed: “v1 yes” (20.11.2026).
- DNS `<PANEL_DOMAIN>` → `<ENTRY_IP>` created by the operator.
- **Quota: reset period + auto-reset/revert** (chosen in the builder round 20.11.2026).
- **Read-only reading of the current production templates permitted** during implementation (20.11.2026).
- **The repo is public: all data anonymized** (directive 20.11.2026).
- **ADRs: create and record them** (request 20.11.2026).
- **Dedicated VPS instead of coexistence** (decision 20.11.2026): 1 vCPU / 1 GB / 10 GB, one IP; do not touch the old server.
- **.gitignore: add everything that must not end up in git** (directive 20.11.2026).

## 12. My decisions (delegated/proposed — edit these)

Svelte stack and embed.FS · TOTP RFC 6238 + QR on the frontend + 10 backup codes · JWT 15min/7d httpOnly · admin init via CLI (no public registration) · quota up+down combined · device_limit hard concurrent, default 0, **auto-revert** when online IPs drop, NAT caveat · expiry auto-disable/enable · v1 users in all enabled inbounds (squads later) · subscription URLs/suffixes/UA fallback · default templates mirror production (RU-direct+adblock) · inbound = form + raw JSON · routing v1 = JSON editor · Reality dest default www.microsoft.com:443 · ports 8443/3002/10811 · panel bind 127.0.0.1:3002 · native Caddy · swap 2 GB · xray v26.11.9 pinned · 30 s polling · daily SQLite backup, 7 copies · UI localized ru/en/de (originally Russian) · stdlib net/http · placeholders instead of real values (§3).
