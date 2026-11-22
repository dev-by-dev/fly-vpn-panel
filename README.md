# fly-vpn-panel

Web Control Panel for VPN — self-hosted panel (Go backend, embedded Svelte 5 SPA) managing xray (VLESS + XHTTP + Reality), users, subscriptions, traffic stats and routing.

## Install on a server

Requirements: Ubuntu 22.04/24.04 or CentOS Stream 9/10, x86_64, root, a domain with an A record pointing at the server, SSH on a known port.

```sh
curl -fsSL https://github.com/dev-by-dev/fly-vpn-panel/releases/latest/download/install.sh -o install.sh
sudo bash install.sh
```

The script asks for the language (ru/en/de), the domain, an optional ACME e-mail and whether to open the firewall. It updates the system, then installs the panel, xray, Caddy with a Let's Encrypt certificate and a systemd unit, generates a random admin password and prints the login details (URL, local URL, login, password) at the end. Every step checks the current state, so running the same command again upgrades without touching data; a flock lock stops a second copy from running at the same time.

Non-interactive: `sudo bash install.sh --yes --domain panel.example.org` (the password is still generated and printed). Other flags: `--lang en`, `--admin <name>`, `--email <addr>`, `--ip <addr>`, `--dry-run`, `--skip-dns`, `--skip-firewall`, `--no-update`, `--force`.

Manual install on other distributions: [docs/deploy-checklist.md](docs/deploy-checklist.md).

## Uninstall

```sh
curl -fsSL https://github.com/dev-by-dev/fly-vpn-panel/releases/latest/download/uninstall.sh -o uninstall.sh
sudo bash uninstall.sh
```

The script asks for the language and makes you type `delete` before anything happens: **the panel data (`/opt/flypanel/data`, including `panel.db` with users, tokens and subscriptions) is deleted without a backup**. Then it removes the panel and xray binaries, both systemd units, our Caddyfile block (a Caddyfile with only our lines is deleted entirely), the `flypanel` and `caddy` users, the firewall rules for 80/443/8443, and the packages the installer added. System updates the installer applied stay: they cannot be rolled back (`--no-update` skips them).

Package removal is exact: `install.sh` writes the list of packages it actually installed to `/opt/flypanel/installed-packages`, and only those are purged — anything that was on the machine before stays. An installation made before that file existed falls back to the extras a base system lacks (`ufw unzip` / `firewalld policycoreutils-python-utils`); `curl`, `ca-certificates` and `iproute2` are never removed. `--keep-packages` skips this step.

Kept on purpose: the SSH rule and everything that was on the machine before the install.

Flags: `--lang ru|en|de`, `--yes` (skip the confirmation word), `--dry-run`, `--skip-firewall`, `--keep-packages`, `--force`.

## Prerequisites

- Go ≥ 1.27
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
| `CADDY_ADMIN_URL` | Caddy admin API root for the metrics scraper (default `http://127.0.0.1:2019`) |

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
