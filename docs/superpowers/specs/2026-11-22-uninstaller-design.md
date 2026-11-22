# flypanel uninstaller — design

## Why

After installing with `scripts/install.sh` the server keeps the user, binaries, units, Caddyfile edits, firewall rules and packages. We need `scripts/uninstall.sh` that returns the machine to its “before install” state: one command, the same three languages, `--dry-run`, idempotency.

## What exactly the installer left behind

Cross-checked against `scripts/install.sh`:

1. the user `flypanel` and `/opt/flypanel/{bin,data,logs,backups}`;
2. `/opt/flypanel/bin/{flypanel,xray,geoip.dat,geosite.dat}`, `/opt/flypanel/.env` (+ `.bak-*`);
3. `/etc/systemd/system/flypanel.service` (enabled, running, xray is its child process);
4. Caddy: `/usr/local/bin/caddy`, user `caddy`, `/var/lib/caddy`, `/etc/systemd/system/caddy.service`, the `# flypanel-managed begin|end` block in `/etc/caddy/Caddyfile`, and possibly the added global `{ metrics }` block;
5. ufw/firewall-cmd rules: SSH port, 80, 443, 8443; `ufw --force enable`;
6. CentOS: `semanage port` for 8443 and 3002;
7. packages (`curl ca-certificates unzip ufw iproute2` / `curl ca-certificates unzip firewalld policycoreutils-python-utils iproute`);
8. `/var/log/flypanel-install.log`.

## Decision on data (user's choice)

`/opt/flypanel/data` is removed entirely, including `panel.db` — no archive, no backup. Protection against an accidental run: interactively you must type the word `delete`; `--yes` skips the question.

## What is deleted and what is not

| Object | Action |
|---|---|
| unit `flypanel`, `caddy` (ours) | stop, disable, `rm`, `daemon-reload` |
| `/opt/flypanel` entirely | `rm -rf` (data included) |
| users `flypanel`, `caddy` | `userdel` (without `-r`: home directories are already gone) |
| `/usr/local/bin/caddy` | delete |
| our block in the Caddyfile | cut out; if nothing but the global `{ metrics }` is left in the file, delete the file and the `/etc/caddy` directory |
| `/var/lib/caddy` | delete |
| firewall rules 80/443/8443 | delete (`ufw delete allow`, `firewall-cmd --permanent --remove-port`) |
| SSH port rule | **not touched** (lockout risk) |
| `ufw`/`firewalld` | **not disabled** (we do not know whether it was on before the install; silently removing protection is worse than leaving it) |
| packages | delete only those the installer added (the list in `$PANEL_DIR/installed-packages`); preinstalled ones are not touched, `--keep-packages` keeps all |
| `semanage port` 8443/3002 | delete (CentOS only, only if the port is labelled `http_port_t`) |
| `/var/log/flypanel-install.log` | delete together with the uninstall log |

Someone else's Caddy configuration (sites that are not ours, a foreign global block) is not touched: if there are no `flypanel-managed` markers, the file stays as is and the script says so.

## Flags

`--lang ru|en|de`, `--yes`, `--dry-run`, `--skip-firewall`, `--keep-packages`, `--force` (keep going on unclear state), `-h`.
`--dry-run` only prints the steps without changing anything; any “scary” state is a warning there, not an exit.

## Script steps

1. root check (before the log is opened), language selection, banner + a warning that data will be lost.
2. Confirmation: the word `delete` (or `--yes`).
3. `systemctl stop flypanel` → xray dies with the cgroup.
4. `systemctl stop caddy`, `systemctl disable flypanel caddy`, `rm` the units, `daemon-reload`.
5. Caddyfile: cut the managed block with awk; if the remainder is empty → delete the file and `/etc/caddy`.
6. `/opt/flypanel` → `rm -rf`; `/usr/local/bin/caddy`, `/var/lib/caddy` → `rm -rf`.
7. `userdel flypanel`, `userdel caddy` (if they exist and hold no processes).
8. Firewall: delete 80/443/8443, keep the SSH port; `--skip-firewall` skips the step.
9. CentOS: `semanage port -d` for 8443 and 3002, if they carry our label.
10. Logs: delete `/var/log/flypanel-install.log`, then the uninstall log itself.
11. Result: what was deleted, what was left on purpose (the SSH rule and everything that existed before the install).

## Non-goals

- not rolling back DNS/ACME certificates;
- not deleting packages that were not there before the install, and not disabling someone else's firewall;
- not working on foreign OSes: Ubuntu 22.04/24.04 / CentOS Stream 9-10, amd64 (same as the installer);
- no “partial” removal by flags (except `--skip-firewall`).

## Verification

1. `bash -n`, shellcheck.
2. Unit tests in `scripts/uninstall_test.sh` (the same `INSTALL_LIB_ONLY`-like sourcing guard trick): cutting out the managed block (present/absent/foreign site), the “delete the Caddyfile or not” decision (only `{ metrics }` → delete), `--dry-run` creates and deletes nothing.
3. A live run on the VM `<host>` (Ubuntu 24.04, v0.1.2 installed): after uninstall — no user, no `/opt/flypanel`, no units, `ss -ltn` without 3002/10811/8443, no Caddyfile, ufw active without our ports; a second run — “nothing to remove”, exit 0.
4. Then a reinstall with the same `install.sh` on the cleaned machine — it must go through from scratch (a check that the uninstaller did not break anything on the system).
5. Publishing: `scripts/uninstall.sh` is added to the CI release assets, the README gets an “Uninstall” section.
## Verification results (VM `<host>`, Ubuntu 24.04)

- `scripts/uninstall_test.sh`: 14 checks, 0 failures (bash 5 on the VM; macOS bash 3.2 cannot do associative arrays).
- Live run on a v0.1.2 install: user `flypanel` removed, `/opt/flypanel` gone, both units removed, ports 3002/10811/8443 free, `https://panel.example.org` does not respond, `ufw` still active, only `22/tcp` left among the rules, services `flypanel` and `caddy` — inactive.
- Second run: “Nothing to remove: flypanel is not installed.”, exit 0. A non-root run: “Root required”, exit 1, before the log is opened.
- Found and fixed on the live run: the `/etc/caddy/Caddyfile.bak-*` backups (made by the installer) prevented the directory from being deleted. Now, when our Caddyfile is deleted, they go with it (covered by test 5).
- A reverse install with the same published `install.sh` (v0.1.2, sha256 `323537c5…`) on the cleaned machine: all 13 steps “done”, HTTPS 200, `flypanel` and `caddy` active, ufw again 22/80/443/8443, a Let's Encrypt certificate issued again, admin created, `flypanel version` → `v0.1.2`.
