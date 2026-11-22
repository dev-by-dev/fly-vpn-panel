# flypanel installer — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** a single `install.sh` that installs the panel, xray and Caddy on a clean Ubuntu 24.04 or CentOS Stream 9/10 and takes it all the way to a working HTTPS address.

**Architecture:** a bash script with a language choice on the first screen, a shared core (user, directories, binary, xray, `.env`, systemd unit, Caddy) and two branches by OS family (apt/ufw, dnf/firewalld/selinux). The panel is installed from GitHub Releases; `admin status` is added to the binary so the installer doesn't drag in `sqlite3`.

**Tech Stack:** bash 5, bash functions and `INSTALL_LIB_ONLY=1` for tests, GitHub Actions, Go 1.27 (only for the `admin status` subcommand).

**Spec:** `docs/superpowers/specs/2026-11-22-installer-design.md`

## Global Constraints

- Target OSes: Ubuntu 24.04 and CentOS Stream 9/10, `x86_64`. Anything else is refused, `--force` with no guarantees.
- The script runs as root, never deletes `data/panel.db` and `data/backups`.
- `/etc/caddy/Caddyfile` and `.env` are backed up as `*.bak-YYYYMMDDHHMM` when overwritten.
- Its own blocks in the Caddyfile live between the markers `# flypanel-managed begin` / `# flypanel-managed end`.
- Versions are pinned: `XRAY_VERSION=26.11.9` (asset `Xray-linux-64.zip`, `.zip.dgst` verification), `CADDY_VERSION=2.11.9` (SHA512 check against `caddy_2.11.9_checksums.txt`).
- All user-facing messages go through the `TXT_RU/TXT_EN/TXT_DE` dictionaries, no gettext and no external files.
- Languages/flags: `--lang ru|en|de`, `--domain`, `--email`, `--admin`, `--ip`, `--yes`, `--dry-run`, `--skip-dns`, `--skip-firewall`, `--force`.
- Nothing is written outside `/opt/flypanel`, `/etc/systemd/system`, `/etc/caddy`, `/usr/local/bin`, `/var/log`; `/etc/os-release` is only read.

## Review Focus

- Re-running on a live panel: ports 3002/8443/10811 are held by our own processes — preflight must pass (test `port_free_or_ours` in Task 2).
- SSH port not detected: the firewall is not enabled, the script exits (test `firewall_allowed` in Task 4).
- `--dry-run` creates no files at all (check in Task 9 on a container).
- A foreign Caddyfile or a foreign site on 80/443 is not overwritten (tests `caddyfile_apply` in Task 5).
- Changing the domain on a re-run replaces the managed block instead of adding a second one (test `caddyfile_apply`).
- An admin already exists → the admin step is skipped, `admin status` returns 0 (Task 1 and Task 6).

---

### Task 1: `flypanel admin status`

**Files:**
- Modify: `cmd/flypanel/admin.go`
- Test: `cmd/flypanel/admin_test.go` (create)

**Interfaces:**
- Consumes: `store.Open(dir)`, `(*store.Store).AdminGet() (*store.Admin, error)`, `store.ErrNotFound`.
- Produces: `func adminStatus(dir string) (bool, error)` — `true` if an admin exists; the CLI `flypanel admin status [--data-dir DIR]` prints `admin exists` and exits 0, otherwise `no admin` and exits 1.

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"testing"

	"flypanel/internal/auth"
	"flypanel/internal/store"
)

func TestAdminStatus(t *testing.T) {
	dir := t.TempDir()
	exists, err := adminStatus(dir)
	if err != nil {
		t.Fatalf("adminStatus(empty): %v", err)
	}
	if exists {
		t.Fatal("adminStatus(empty) = true, want false")
	}

	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("store.Open(): %v", err)
	}
	if err := auth.AdminInit(st, "admin", "TestPassword-123"); err != nil {
		t.Fatalf("AdminInit(): %v", err)
	}
	st.Close()

	exists, err = adminStatus(dir)
	if err != nil {
		t.Fatalf("adminStatus(seeded): %v", err)
	}
	if !exists {
		t.Fatal("adminStatus(seeded) = false, want true")
	}
}
```

- [ ] **Step 2: Confirm the test fails**

Run: `go test ./cmd/flypanel/ -run TestAdminStatus`
Expected: FAIL — `undefined: adminStatus`.

- [ ] **Step 3: Implement**

In `cmd/flypanel/admin.go` split `adminCommand` into subcommands and add the function. The data path comes from `config.Load()` + `--data-dir`, as in `init`.

```go
func adminStatus(dir string) (bool, error) {
	st, err := store.Open(dir)
	if err != nil {
		return false, err
	}
	defer st.Close()
	_, err = st.AdminGet()
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
```

`status` CLI branch: its own `flag.NewFlagSet("admin status")`, printing `admin exists` / `no admin`, `os.Exit(0)` / `os.Exit(1)`; add the `admin status` line to `usage()` in `main.go`.

- [ ] **Step 4: Run the test**

Run: `go test ./cmd/flypanel/ -run TestAdminStatus -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/flypanel/admin.go cmd/flypanel/admin_test.go cmd/flypanel/main.go
git commit -m "feat(admin): report whether an admin account exists"
```

---

### Task 2: script skeleton, language, preflight

**Files:**
- Create: `scripts/install.sh`
- Test: `scripts/install_test.sh`

**Interfaces:**
- Consumes: nothing from Go.
- Produces (functions available under `INSTALL_LIB_ONLY=1`): `t <K>`; `detect_family` → prints `apt|dnf|unsupported`, reads `/etc/os-release`; `valid_domain <d>` → 0/1; `port_free_or_ours <port>` → 0 if the port is free or held by a process from `flypanel|xray|caddy`; `detect_ssh_port` → prints the port or nothing; `have <cmd>`; `run <cmd...>` (in dry-run it prints, otherwise it executes).

- [ ] **Step 1: Write the failing test harness**

`scripts/install_test.sh`: `set -euo pipefail`, `INSTALL_LIB_ONLY=1 source scripts/install.sh`, a failure counter, functions:

- `detect_family` against a substituted `/etc/os-release` (the `OS_RELEASE_FILE` variable) → `ubuntu`+`24.04` = `apt`; `centos`+`9` = `dnf`; `debian`+`12` = `unsupported`; `centos`+`7` = `unsupported`.
- `valid_domain panel.example.org` = 0; `valid_domain "panel"` = 1; `valid_domain "example.com"` = 1; `valid_domain "<panel>"` = 1.
- `port_free_or_ours`: occupy the port with `python3 -c 'import socket,time; s=socket.socket(); s.bind(("127.0.0.1",P)); s.listen(); time.sleep(2)'` — a foreign process → 1; a port with no listener → 0. We don't write a test for "our process": faking the process name is fragile, the behaviour is covered by a real re-run in Task 10.
- `detect_ssh_port` with `SSHD_T_CMD=/bin/false` (empty output) → an empty string.

- [ ] **Step 2: Run — fail**

Run: `bash scripts/install_test.sh`
Expected: FAIL — `scripts/install.sh: No such file or directory`.

- [ ] **Step 3: Implement the skeleton**

`scripts/install.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

INSTALL_LOG=/var/log/flypanel-install.log
XRAY_VERSION=26.11.9
CADDY_VERSION=2.11.9
OS_RELEASE_FILE=${OS_RELEASE_FILE:-/etc/os-release}
SSHD_T_CMD=${SSHD_T_CMD:-sshd -T}

# --- i18n: three parallel arrays, index constants ---
TXT_RU=(...); TXT_EN=(...); TXT_DE=(...)
K_WELCOME=0; ...
t() { local k=$1; case $LANG_SEL in ru) printf '%s' "${TXT_RU[$k]}" ;; ... esac; }

detect_family() {
  [ -r "$OS_RELEASE_FILE" ] || { echo unsupported; return; }
  . "$OS_RELEASE_FILE"
  case "${ID:-}:${VERSION_ID:-}" in
    ubuntu:24.04) echo apt ;;
    centos:9|centos:10|centos:9.*|centos:10.*) echo dnf ;;
    *) echo unsupported ;;
  esac
}

valid_domain() { [[ $1 =~ ^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$ ]] && [[ $1 != example.com ]] && [[ $1 != *'<'* ]]; }

have() { command -v "$1" >/dev/null 2>&1; }

run() { if $DRY_RUN; then printf '  [dry-run] %s\n' "$*"; else "$@"; fi; }

port_free_or_ours() { ... ss -ltnp ... }

detect_ssh_port() { $SSHD_T_CMD 2>/dev/null | awk '/^port /{print $2; exit}'; }

main() { ... }   # below, Task 3+
[[ ${INSTALL_LIB_ONLY:-0} == 1 ]] || main "$@"
```

Plus the real `main` skeleton: `parse_args`, language selection (default from `$LANG`), `preflight` (root, `x86_64`, systemd, family, container, disk, ports, IP, DNS, ssh port), a confirmation screen. The texts go into all three dictionaries.

- [ ] **Step 4: Run the tests**

Run: `bash scripts/install_test.sh`
Expected: `all checks passed`.

- [ ] **Step 5: Commit**

```bash
git add scripts/install.sh scripts/install_test.sh
git commit -m "feat(installer): skeleton, i18n and preflight"
```

---

### Task 3: shared install core

**Files:**
- Modify: `scripts/install.sh`
- Test: `scripts/install_test.sh` (`env_file_write` checks)

**Interfaces:**
- Consumes: `run`, `have`, `t`, `FAMILY`, `IP`, `DOMAIN`, `ADMIN_NAME`, `EMAIL`.
- Produces: `env_file_write <path>` → writes `.env` with a backup; `install_packages`; `create_user_dirs`; `install_panel`; `install_xray`; `install_unit`; `wait_https <domain>` → 0/1.

- [ ] **Step 1: Test for `.env`**

In `scripts/install_test.sh`: `env_file_write` into an empty directory creates a file with `LISTEN_ADDR=127.0.0.1:3002`, `DATA_DIR=<dir>/data`, `ENTRY_IP=203.0.113.9`, `PANEL_DOMAIN=panel.example.org`, `COOKIE_SECURE=true`, mode `600`; a second call creates `*.bak-*` and does not lose old values outside the listed keys.

Run: `bash scripts/install_test.sh` → FAIL (function not defined).

- [ ] **Step 2: Implement the steps**

- `install_packages`: Ubuntu `apt-get update` + `curl ca-certificates unzip ufw iproute2`; CentOS `dnf install -y curl ca-certificates unzip firewalld policycoreutils-python-utils iproute` + `systemctl enable --now firewalld`.
- `create_user_dirs`: `useradd --system --home-dir /opt/flypanel --shell /usr/sbin/nologin flypanel` (if absent), `mkdir -p bin data logs backups`.
- `install_panel`: download `https://github.com/dev-by-dev/fly-vpn-panel/releases/latest/download/flypanel-linux-amd64` and `sha256sums.txt` into `mktemp -d`, `sha256sum -c` against the line for this file, install to `bin/flypanel.new`, `chown flypanel:flypanel`, `chmod 755`, `mv` over `bin/flypanel`.
- `install_xray`: `Xray-linux-64.zip` + `.zip.dgst` from `https://github.com/XTLS/Xray-core/releases/download/v${XRAY_VERSION}/`, verify `SHA2-256=`, unpack `xray geoip.dat geosite.dat` into `bin/`; if `bin/xray` already exists at a different version — warn and leave it alone without `--force`.
- `env_file_write`: backup `.bak-$(date +%Y%m%d%H%M)`, write exactly the ten keys from the spec, `chmod 600`, `chown flypanel:flypanel`.
- `install_unit`: the unit from the checklist (`PrivateTmp=true`, `ProtectSystem=strict`, `ReadWritePaths=/opt/flypanel`, `SupplementaryGroups=systemd-journal`, `Restart=on-failure`), `systemctl daemon-reload`, `enable`, `restart` (not `--now`, the panel is brought up after `.env`).
- `wait_https`: up to 90 s of `curl -fsS "https://$1/"` checking for `assets/index-`.

- [ ] **Step 3: Run the tests**

Run: `bash scripts/install_test.sh`
Expected: PASS, including `env_file_write`.

- [ ] **Step 4: Commit**

```bash
git add scripts/install.sh scripts/install_test.sh
git commit -m "feat(installer): panel, xray, env and unit steps"
```

---

### Task 4: firewall and SELinux

**Files:**
- Modify: `scripts/install.sh`
- Test: `scripts/install_test.sh` (`firewall_allowed` check)

**Interfaces:**
- Consumes: `FAMILY`, `SSH_PORT`, `ENABLE_FIREWALL`.
- Produces: `firewall_allowed <port>` → 0 if there is an ssh port; `configure_firewall`; `configure_selinux`.

- [ ] **Step 1: Test**

`firewall_allowed` with `SSH_PORT=""` → 1 (must not be enabled), with `SSH_PORT=2222` → 0. `configure_firewall` with `SSH_PORT=""` does not call `run` (intercept `run` by substitution in the test: `run() { echo "RUN $*"; }`).

- [ ] **Step 2: Implement**

`configure_firewall`: Ubuntu — `ufw allow "$SSH_PORT/tcp"`, then 80, 443, 8443, `ufw --force enable`; CentOS — `firewall-cmd --permanent --add-port="$SSH_PORT/tcp"` and the same ports, `--reload`. Order: SSH first, enabling last. If `detect_ssh_port` is empty — warn, print the commands for manual use, exit (or continue with `--skip-firewall`).

`configure_selinux`: only if `getenforce` = `Enforcing`; `semanage port -a -t http_port_t -p tcp 8443`, `3002` (with `-m` when the label is taken), `systemctl restart caddy flypanel`; errors are a warning, not an exit.

- [ ] **Step 3: Tests**

Run: `bash scripts/install_test.sh` → PASS.

- [ ] **Step 4: Commit**

```bash
git commit -am "feat(installer): firewall and selinux branches"
```

---

### Task 5: Caddy with markers

**Files:**
- Modify: `scripts/install.sh`
- Test: `scripts/install_test.sh` (`caddyfile_apply` tests)

**Interfaces:**
- Produces: `caddyfile_apply <path> <domain>` → 0 on success; 1 with a message if the config is foreign; `install_caddy`.

- [ ] **Step 1: Tests (RED)**

`caddyfile_apply` on an empty file → a block with markers, `{ metrics }` and `reverse_proxy 127.0.0.1:3002`; a second call with a different domain → one managed block, the new domain inside, the old one absent; a file with a foreign site on `:80` and no markers → 1, the file unchanged; a file with a foreign global block `{ email a@b.c }` → 1.

- [ ] **Step 2: Implement**

Download `caddy_${CADDY_VERSION}_linux_amd64.tar.gz` + `caddy_${CADDY_VERSION}_checksums.txt` (SHA512), install to `/usr/local/bin/caddy`; the `caddy` user, directories; a `caddy.service` unit; `caddyfile_apply` (awk/python-free: `sed`/heredoc, markers mandatory); `caddy validate --config /etc/caddy/Caddyfile`; `systemctl enable caddy` + `restart` (or `reload` if active).

- [ ] **Step 3: Tests**

Run: `bash scripts/install_test.sh` → PASS (4 Caddyfile cases).

- [ ] **Step 4: Commit**

```bash
git commit -am "feat(installer): caddy with managed config markers"
```

---

### Task 6: admin, summary, step order

**Files:**
- Modify: `scripts/install.sh`

**Interfaces:**
- Consumes: `flypanel admin status` (Task 1).
- Produces: `admin_step`; the final summary.

- [ ] **Step 1: Implement**

`admin_step`: `runuser -u flypanel -- env FLYPANEL_ENV=/opt/flypanel/.env /opt/flypanel/bin/flypanel admin status` → code 0 — print «админ уже есть», skip; otherwise, if not `--yes`, `runuser -u flypanel -- env ... admin init` (the terminal stays with the user); `--yes` — skip with a reminder. A failed `admin init` is a warning, the script's exit code is 1 at the end.

`main` order: args → language → preflight → questions → confirmation → packages → user/dirs → panel → xray → env → chown → unit → firewall → selinux → caddy → wait_https → admin → summary.

- [ ] **Step 2: Check end to end in dry-run**

Run: `sudo bash scripts/install.sh --dry-run --force --domain panel.example.org --admin admin --lang ru`
Expected: all 13 steps printed in the right order, not a single change on disk (`ls /opt/flypanel` → no such directory), code 0.

- [ ] **Step 3: Commit**

```bash
git commit -am "feat(installer): admin handoff and summary"
```

---

### Task 7: documentation

**Files:**
- Modify: `README.md`, `docs/deploy-checklist.md`

- [ ] **Step 1:** In `README.md` — an «Установка» section: a single command to download the script (not `curl | bash`), supported OSes, what it will ask, where the log is, how to update.
- [ ] **Step 2:** In `docs/deploy-checklist.md` — a header: «для ручной установки; автоматическая — `scripts/install.sh`», and a note that the script performs the same steps.
- [ ] **Step 3: Commit**

```bash
git add README.md docs/deploy-checklist.md
git commit -m "docs: installer quick start"
```

---

### Task 8: release workflow

**Files:**
- Create: `.github/workflows/release.yml`

- [ ] **Step 1: Write the workflow**

Trigger `push: tags: ['v*']`; steps: checkout, `actions/setup-node@v4` (node 20), `actions/setup-go@v5` with `go-version-file: go.mod`, `make cross`, `sha256sum bin/flypanel-linux-amd64 scripts/install.sh > sha256sums.txt`, `softprops/action-gh-release@v2` with `files: bin/flypanel-linux-amd64, scripts/install.sh, sha256sums.txt`, `permissions: contents: write`.

**Inside `sha256sums.txt` the names must not include `bin/`** — otherwise `sha256sum -c` in the script won't find the file; do `cd bin && sha256sum flypanel-linux-amd64` and add `scripts/install.sh` on a separate line.

- [ ] **Step 2: Check the YAML**

Run: `python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/release.yml'))"`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "ci: release workflow for tags"
```

---

### Task 9: first release (requires explicit consent to push)

**Files:** none.

- [ ] **Step 1:** `git checkout main && git merge --no-edit dev` — run `make test` and `make web-test`.
- [ ] **Step 2:** `git push origin main`.
- [ ] **Step 3:** `git tag v0.1.0 && git push origin v0.1.0`; wait for a green Actions run.
- [ ] **Step 4: Check the assets**

Run: `curl -fsSL https://github.com/dev-by-dev/fly-vpn-panel/releases/latest/download/sha256sums.txt`
Expected: two lines, named `flypanel-linux-amd64` and `install.sh`.

---

### Task 10: live runs

- [ ] **Step 1: Ubuntu (<host>, public IP, real domain)** — `--lang en`; expectation: HTTPS 200, the user created, `/sub/<token>` in 4 formats, «Мониторинг → Логи» and «Caddy» return data.
- [ ] **Step 2: Re-run** on the same machine: «всё уже стоит», the admin is intact, the domain is not duplicated, the panel restarted.
- [ ] **Step 3: CentOS Stream 9 (VM or VPS)** with `--skip-dns`: packages, `firewall-cmd --list-ports` contains 80/443/8443, `getenforce`, no fresh AVCs, the panel on `127.0.0.1:3002`; the HTTPS step fails with a clear message — that is expected.
- [ ] **Step 4: Negative cases** — wrong domain (DNS doesn't match) → exit before any changes; port 80 busy → exit with the process name; not root → error in the selected language.
- [ ] **Step 5:** Record the results in `docs/superpowers/specs/2026-11-22-installer-design.md` (§ Verification) and commit.