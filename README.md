# fly-vpn-panel

Web Control Panel for VPN — self-hosted panel (Go backend, embedded Svelte 5 SPA) managing xray (VLESS + XHTTP + Reality), users, subscriptions, traffic stats and routing.

## Prerequisites

- Go ≥ 1.22
- Node.js ≥ 20.19 (with npm) — for the web dashboard build
- make

## Build

```sh
make build   # web: npm ci && npm run build; then go build -o bin/flypanel (SPA embedded)
make cross   # cross-compile: bin/flypanel-linux-amd64 (GOOS=linux GOARCH=amd64)
make test    # go test ./...
make web-test# web unit tests (vitest)
```

`make build` embeds the built SPA (`web/dist`) into the binary via `go:embed`, so the single `bin/flypanel` binary serves the dashboard — no separate static hosting needed.

## Configure

Copy the template and fill in your values (chmod 600; never commit a real `.env`):

```sh
cp .env.example .env
```

Keys (path selectable via the `FLYPANEL_ENV` env var, default `./.env`):

| Key | Meaning |
|-----|---------|
| `DATA_DIR` | SQLite data directory (default `./data`) |
| `LISTEN_ADDR` | HTTP listen address (default `127.0.0.1:3002`) |
| `XRAY_BIN` | Path to the xray binary (default `./bin/xray`) |
| `ENTRY_IP` | Public entry server IP |
| `EGRESS_IP` | Optional egress server IP (empty = egress via entry) |
| `PANEL_DOMAIN` | Domain the panel is served on |
| `POLL_INTERVAL_SEC` | Xray stats poll interval (default 30) |
| `COOKIE_SECURE` | `true` when served over HTTPS (Secure cookies) |

## Run

```sh
# 1. create the admin account (username arg; password via stdin, min 12 chars)
printf 'your-password-here\n' | ./bin/flypanel admin init --password-stdin admin

# 2. serve the panel (dashboard + API on LISTEN_ADDR)
./bin/flypanel serve
```

The panel serves the dashboard and API on `LISTEN_ADDR`; user subscriptions are served at `/sub/{token}`.

## Smoke test

Local end-to-end check (builds, creates an admin, serves on 127.0.0.1:3002 with a stub xray binary, logs in, verifies the API and the embedded SPA):

```sh
scripts/smoke.sh   # prints PASS on success, non-zero exit on failure
```

## Server layout (suggested)

```
/opt/flypanel        # binary, .env, web assets not needed (embedded)
/opt/flypanel/data   # SQLite database
/opt/flypanel/bin/xray
```
