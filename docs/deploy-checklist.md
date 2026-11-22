# Deploy checklist

> Automatic install on Ubuntu 24.04 or CentOS Stream 9/10: see the release asset `install.sh` and the README. This document is the manual path and the reference for what the script does.

Generic playbook for deploying flypanel on a dedicated Linux VPS. Real values
(`<ENTRY_IP>`, `<PANEL_DOMAIN>`, `<EGRESS_IP>`, server alias, provider) live in
the operator's private runbook — **never** in this repo. See ADR-0008 (dedicated
VPS) and `docs/superpowers/specs/2026-11-20-fly-vpn-panel-design.md` §8.

Placeholders: `<ENTRY_IP>` public VPS IP (entry=egress when single IP),
`<PANEL_DOMAIN>` panel/subscription hostname, `<EGRESS_IP>` optional separate
egress IP (empty → `sendThrough` omitted).

## 0. Prerequisites
- Dedicated VPS, Ubuntu 24.04 LTS, ≥1 vCPU / 1 GB RAM / 10 GB disk.
- Ports free: 80, 443, 8443 (VPN), 3002 (panel loopback), 10811 (xray API loopback).
  Check `ss -ltnp`. Do NOT touch the provider agent's ports.
- SSH access via operator tooling only; explicit approval per action; never
  change the SSH port via sshd config.

## 1. Swap (1 GB RAM boxes)
```
fallocate -l 2G /swapfile && chmod 600 /swapfile && mkswap /swapfile && swapon /swapfile
echo '/swapfile none swap sw 0 0' >> /etc/fstab
```

## 2. Files
- `useradd --system --home-dir /opt/flypanel --shell /usr/sbin/nologin flypanel`
- `mkdir -p /opt/flypanel/{bin,data,logs,backups}`
- Build on the dev machine: `make cross` → `bin/flypanel-linux-amd64`
  (ELF x86-64). **`make cross` builds the web SPA first** (see pitfall #5).
- Upload binary → `/opt/flypanel/bin/flypanel`; verify sha256 local==remote;
  `chmod +x`.
- xray-core **v26.11.9** official release. NOTE: the linux amd64 asset is
  `Xray-linux-64.zip` (NOT `Xray-linux-amd64.zip`); verify the `.zip.dgst`
  `SHA2-256=` line against `sha256sum`. Extract `xray`, `geoip.dat`,
  `geosite.dat` → `/opt/flypanel/bin/`; `chmod +x xray`; `xray version` must
  report 26.11.9.
- `/opt/flypanel/.env` (chmod 600, owner flypanel):
  ```
  LISTEN_ADDR=127.0.0.1:3002
  DATA_DIR=/opt/flypanel/data
  XRAY_BIN=/opt/flypanel/bin/xray
  ENTRY_IP=<ENTRY_IP>
  EGRESS_IP=
  PANEL_DOMAIN=<PANEL_DOMAIN>
  POLL_INTERVAL_SEC=30
  COOKIE_SECURE=true
  ```
- `chown -R flypanel:flypanel /opt/flypanel`

## 3. systemd
`/etc/systemd/system/flypanel.service`:
```
[Unit]
Description=flypanel VPN control panel
After=network.target

[Service]
Type=simple
User=flypanel
Group=flypanel
SupplementaryGroups=systemd-journal
WorkingDirectory=/opt/flypanel
Environment=FLYPANEL_ENV=/opt/flypanel/.env
ExecStart=/opt/flypanel/bin/flypanel serve
NoNewPrivileges=true
ProtectSystem=strict
PrivateTmp=true
ReadWritePaths=/opt/flypanel
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
```
`systemctl daemon-reload && systemctl enable --now flypanel`.
**`PrivateTmp=true` is REQUIRED** — `ProtectSystem=strict` makes `/tmp`
read-only and xray config validation writes a temp file there (pitfall #2).

`SupplementaryGroups=systemd-journal` lets the panel read `journalctl` for the
«Мониторинг → Логи» view. Without it the page shows
«Журнал systemd недоступен» (backend 502 `journal_unavailable`) and everything
else keeps working. Apply with `systemctl daemon-reload && systemctl restart flypanel`.

## 4. Admin
Operator runs interactively over SSH (password never in chat/logs):
```
sudo -u flypanel FLYPANEL_ENV=/opt/flypanel/.env /opt/flypanel/bin/flypanel admin init
```
Username + password ≥12 chars. 2FA (TOTP) optional, enable later in Settings UI.

## 5. UFW
```
ufw allow 22/tcp; ufw allow 80/tcp; ufw allow 443/tcp; ufw allow 8443/tcp
# keep the provider agent's public port open if it had one (e.g. 8080/tcp)
ufw default deny incoming; ufw default allow outgoing; ufw --force enable
```
3002/10811 are loopback-only — no external rules needed. Allow SSH BEFORE enable.

## 6. DNS + Caddy (native)
- Operator points `<PANEL_DOMAIN>` A record → `<ENTRY_IP>`. Verify via DoH
  (`https://dns.google/resolve?name=<PANEL_DOMAIN>&type=A`).
- Install Caddy from the official cloudsmith repo (native, not Docker).
- `/etc/caddy/Caddyfile` (backup first: `cp -a Caddyfile Caddyfile.bak-YYYYMMDD`):
  ```
  {
      metrics
  }

  <PANEL_DOMAIN> {
      log
      reverse_proxy 127.0.0.1:3002
  }
  ```
  The global `metrics` block enables Caddy's own Prometheus counters — the panel
  scrapes them every 15 s from `http://127.0.0.1:2019/metrics` into SQLite for
  «Мониторинг → Caddy». Without it the admin endpoint still answers but has no
  `caddy_http_requests_total`, and the view stays at «Данных пока нет».
  `log` writes the JSON access log to stderr → journald, where the logs view reads it.
- `caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile`
- `systemctl reload caddy` (breaks established panel connections; on a fresh
  single-site server no quiet window needed). Caddy auto-obtains Let's Encrypt
  cert via HTTP-01 (port 80). Verify `curl -sI https://<PANEL_DOMAIN>/` → 200
  and the body is the built SPA (hashed `/assets/index-*.js`), not the placeholder.
- Metrics check: `curl -s http://127.0.0.1:2019/metrics | grep -c caddy_http_requests_total`
  → non-zero after a request to the panel. `CADDY_ADMIN_URL` overrides the default
  `http://127.0.0.1:2019` if Caddy's admin API listens elsewhere.
- Logs check: «Мониторинг → Логи» shows lines for `server`, `caddy` and `xray`
  (the last one appears only after xray logs a warning/error).

## 7. E2E verification
- Create a test user in the UI; fetch `/sub/<token>` in all 4 formats
  (base64 / json / mihomo / clash) — each must be valid.
- Run an xray client from the json config; `curl --socks5-hostname` through it
  → `https://api.ipify.org` must return `<ENTRY_IP>` (or `<EGRESS_IP>` if set).
- RU-direct routing: a `.ru` site goes `[socks -> direct]`, foreign goes
  `[socks >> proxy]` in the client access log.
- Visibility: after a poller tick (30 s) the dashboard shows the client device
  (IP + last_seen) and traffic samples. `xray api statsquery --server=127.0.0.1:10811
  -pattern ""` must return non-empty counters.
- Quota: set user quota to 1 MB, exceed it → poller disables (reason=quota) and
  hot-removes the user (`xray api inboundusercount` → 0); reset quota → poller
  re-enables and hot-adds (count → 1).

## 8. Rollback
`systemctl stop flypanel`; `ufw delete allow 8443/tcp`; remove the Caddy site
block + `systemctl reload caddy`; revert DNS if needed. `/opt/flypanel` data is
left untouched.

## Known pitfalls (found + fixed during first deploy)
1. **serve must generate the initial xray config** before starting xray
   (`applyInitialXrayConfig` → `Applier.Apply`), else xray crash-loops on a
   missing `data/xray.json`.
2. **`PrivateTmp=true`** in the unit — `ProtectSystem=strict` makes `/tmp`
   read-only; xray `run -test` validation writes a temp file there.
3. **VLESS inbound settings require `"decryption":"none"`** — xray 26.x rejects
   the config otherwise (exit 23). `GenerateConfig` injects it for vless.
4. **API routing rule is mandatory**: `{type:field, inboundTag:["api"],
   outboundTag:"api"}`. Without it the dokodemo-door api inbound falls through
   to freedom → gRPC "error reading server preface". `GenerateConfig` prepends it.
5. **`make cross` must build the web SPA first** — the binary embeds `web/dist`
   at `go build` time; building cross with the placeholder restored ships a
   "build me" page instead of the real SPA.
6. **Top-level `"stats": {}` is required** — xray loads the stats manager app
   only when the config has a `stats` object; without it all counters are empty
   (no traffic/device visibility) even with `policy.statsUser*` set.
7. **`GetAllOnlineUsers` returns full counter names** `user>>>email>>>online`;
   the client must extract the bare email before calling `GetStatsOnlineIpList`
   (which re-wraps it), else devices stay empty and device-limit enforcement
   sees zero online IPs.

8. **Caddy HTTP metrics are off until `{ metrics }` is added** to the global
   Caddyfile block. The panel cannot edit `/etc/caddy` (ReadWritePaths) — this is
   an operator step, and the Caddy view stays empty until it is done.

## Notes
- xray writes warnings and errors to `data/xray-error.log` (truncated at 5 MB on
  the next config apply); stdout/stderr still go to journald.
- Caddy logs go to journald; the panel reads them through the systemd-journal group.
- Validate captures CombinedOutput (xray writes config errors to stdout).
- Daily SQLite backup via `VACUUM INTO data/backups/panel-YYYYMMDD.db` (keep 7).
- Poller startup races xray listen for one tick (connection refused) then
  connects; this is expected, not a fault.
