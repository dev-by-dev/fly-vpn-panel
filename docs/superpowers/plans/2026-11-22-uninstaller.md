# Uninstaller Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `scripts/uninstall.sh` removes everything `scripts/install.sh` created (panel, xray, our Caddy, Caddyfile managed block, firewall rules, users, logs) and leaves the machine fit for a clean reinstall.

**Architecture:** One self-contained bash script, same shape as the installer: three associative message dictionaries (ru/en/de), `say/step/ok/run` helpers, `--dry-run`, idempotent guarded steps, `UNINSTALL_LIB_ONLY=1` sourcing guard so `scripts/uninstall_test.sh` can unit-test the pure parts (Caddyfile stripping, confirmation word, dry-run guards) without root.

**Tech Stack:** bash 5, awk, systemd, ufw/firewall-cmd, semanage (CentOS), shellcheck, GitHub Actions release workflow.

**Spec:** `docs/superpowers/specs/2026-11-22-uninstaller-design.md`

## Global Constraints

- Target Ubuntu 24.04 / CentOS Stream 9-10, x86_64; root required (except `--dry-run`).
- Data is destroyed without archive — but only after the exact word `delete` (or `--yes`).
- Never remove packages, never disable ufw/firewalld, never delete the SSH firewall rule.
- Foreign Caddy config untouched: no `flypanel-managed` markers → leave the file alone.
- All user-facing text via `MSG_RU`/`MSG_EN`/`MSG_DE`; unknown key falls back to ru.
- `--dry-run` changes nothing at all and treats scary states as warnings.
- Idempotent: second run prints "nothing to remove" and exits 0.
- Log: `/var/log/flypanel-uninstall.log` via `tee`, removed as the last step.

## Review Focus

- Re-run on a clean machine must exit 0 and delete nothing (no `rm` errors leaking).
- `userdel` while a process still holds the account must not abort the script mid-way.
- Caddyfile with only `{ metrics }` left must be removed; with a foreign site must survive.
- `--dry-run` must not stop units, not touch files, not run `userdel`.
- Firewall step must never remove the SSH rule even if the port list looks wrong.

---

### Task 1: Skeleton, i18n, confirmation, dry-run helpers

**Files:**
- Create: `scripts/uninstall.sh`
- Create: `scripts/uninstall_test.sh`

**Interfaces:**
- Produces: `t KEY`, `say KEY`, `sayf KEY ARG...`, `step KEY`, `ok`, `run CMD...`, `word_ok WORD` (0 when input is exactly `delete`), `remove_path PATH` (respects `DRY_RUN`), `UNINSTALL_LIB_ONLY` guard, vars `PANEL_DIR`, `ACCESS_PORT=3002`, `UNINSTALL_LOG`, `DRY_RUN`, `ASSUME_YES`, `SKIP_FIREWALL`, `FORCE`, `LANG_SEL`, `FAMILY`.

- [ ] **Step 1: Write the failing tests**

`scripts/uninstall_test.sh`:

```bash
#!/usr/bin/env bash
set -u
UNINSTALL_LIB_ONLY=1 source "$(dirname "$0")/uninstall.sh"
set +e
fails=0
check() { if eval "$2"; then printf 'ok   %s\n' "$1"; else printf 'FAIL %s\n' "$1"; fails=$((fails+1)); fi; }

check "word_ok accepts delete" "word_ok delete; [ \$? -eq 0 ]"
check "word_ok rejects DELETE" "word_ok DELETE; [ \$? -ne 0 ]"
check "word_ok rejects yes" "word_ok yes; [ \$? -ne 0 ]"

tmp=$(mktemp -d); printf 'x\n' > "$tmp/keep"
DRY_RUN=true
remove_path "$tmp/keep" >/dev/null
check "dry-run keeps file" "[ -f '$tmp/keep' ]"
DRY_RUN=false
remove_path "$tmp/keep" >/dev/null
check "real run removes file" "[ ! -f '$tmp/keep' ]"
check "remove_path tolerates missing" "remove_path '$tmp/gone' >/dev/null"
rm -rf "$tmp"

printf '%s\n' "$fails failures"
[ "$fails" -eq 0 ]
```

- [ ] **Step 2: Run to verify it fails**

Run: `bash scripts/uninstall_test.sh`
Expected: FAIL — `uninstall.sh: No such file or directory`.

- [ ] **Step 3: Write the skeleton**

`scripts/uninstall.sh`: shebang, `set -euo pipefail`, header comment (`# flypanel uninstaller: Ubuntu 24.04 / CentOS Stream 9-10, x86_64.` + `# Removes what install.sh created. Panel data is deleted.`), globals as in Interfaces, `declare -A MSG_RU/MSG_EN/MSG_DE` with keys `welcome`, `lang_ask`, `need_root`, `warn_data`, `ask_delete`, `wrong_word`, `aborted`, `step_units`, `step_caddy`, `step_files`, `step_user`, `step_firewall`, `step_selinux`, `step_logs`, `ok`, `skip`, `dry`, `nothing`, `done`, `keep_notes`, `err_user_busy`, and `t/say/sayf/step/ok/run` copied in behaviour from `scripts/install.sh` (associative arrays; `run` prints `[dry-run] cmd` under `DRY_RUN`), plus:

```bash
word_ok() { [ "${1:-}" = delete ]; }
remove_path() { [ -e "$1" ] || return 0; run rm -rf "$1"; }
```

last line `[[ ${UNINSTALL_LIB_ONLY:-0} == 1 ]] || main "$@"` with a stub `main() { :; }` for now.

- [ ] **Step 4: Verify**

Run: `bash -n scripts/uninstall.sh && bash scripts/uninstall_test.sh`
Expected: `0 failures`, exit 0.

- [ ] **Step 5: Commit**

```bash
git add scripts/uninstall.sh scripts/uninstall_test.sh
git commit -m "feat(uninstall): script skeleton with tests"
```

---

### Task 2: Caddyfile managed-block stripping

**Files:**
- Modify: `scripts/uninstall.sh`
- Modify: `scripts/uninstall_test.sh`

**Interfaces:**
- Produces: `caddyfile_strip PATH` → 0 = block removed, file kept; 1 = file was only ours and is deleted (dir too); 2 = no markers, file untouched.

- [ ] **Step 1: Write the failing tests**

Append to `scripts/uninstall_test.sh` before the summary:

```bash
tmp=$(mktemp -d)
caddyfile_strip_markers() { printf '%s\n' '{\n\tmetrics\n}' '' '# flypanel-managed begin' 'panel.example.org {' '	log' '	reverse_proxy 127.0.0.1:3002' '}' '# flypanel-managed end'; }

# 1: only our global block and our site -> file and dir go away
mkdir -p "$tmp/a"; printf '%s\n' '{\n\tmetrics\n}' > "$tmp/a/Caddyfile"
printf '\n# flypanel-managed begin\npanel.example.org {\n\tlog\n\treverse_proxy 127.0.0.1:3002\n}\n# flypanel-managed end\n' >> "$tmp/a/Caddyfile"
caddyfile_strip "$tmp/a/Caddyfile" >/dev/null 2>&1; rc=$?
check "only ours -> removed (rc 1)" "[ $rc -eq 1 ] && [ ! -f '$tmp/a/Caddyfile' ]"

# 2: foreign site survives, our block goes
mkdir -p "$tmp/b"; printf 'example.org {\n\trespond hi\n}\n\n# flypanel-managed begin\npanel.example.org {\n\tlog\n}\n# flypanel-managed end\n' > "$tmp/b/Caddyfile"
caddyfile_strip "$tmp/b/Caddyfile" >/dev/null 2>&1; rc=$?
check "foreign site kept (rc 0)" "[ $rc -eq 0 ] && grep -q 'example.org' '$tmp/b/Caddyfile' && ! grep -q flypanel-managed '$tmp/b/Caddyfile'"

# 3: no markers -> untouched
mkdir -p "$tmp/c"; printf 'example.org {\n\trespond hi\n}\n' > "$tmp/c/Caddyfile"
caddyfile_strip "$tmp/c/Caddyfile" >/dev/null 2>&1; rc=$?
check "no markers -> untouched (rc 2)" "[ $rc -eq 2 ] && grep -q 'example.org' '$tmp/c/Caddyfile'"
rm -rf "$tmp"
```

- [ ] **Step 2: Run to verify it fails**

Run: `bash scripts/uninstall_test.sh`
Expected: FAIL on all three — `caddyfile_strip: command not found`.

- [ ] **Step 3: Implement `caddyfile_strip`**

awk drops lines from `# flypanel-managed begin` through `# flypanel-managed end`. If no marker was seen, return 2 without writing. Then decide the remainder: using a second awk that tracks a leading `{ ... }` block, "meaningful" means a line with a non-comment token outside that block (or a site block starts). Nothing meaningful → `remove_path` the file and its directory, return 1. Otherwise install the stripped file atomically (`tmp` in the same dir, `mv`), return 0.

- [ ] **Step 4: Verify**

Run: `bash -n scripts/uninstall.sh && bash scripts/uninstall_test.sh`
Expected: `0 failures`.

- [ ] **Step 5: Commit**

```bash
git add scripts/uninstall.sh scripts/uninstall_test.sh
git commit -m "feat(uninstall): strip managed caddy block"
```

---

### Task 3: Removal steps (units, files, users)

**Files:**
- Modify: `scripts/uninstall.sh`

**Interfaces:**
- Produces: `stop_units`, `remove_caddy_config`, `remove_files`, `remove_users`, each honouring `DRY_RUN`; used by `main`.

- [ ] **Step 1: Implement `stop_units`**

`step step_units`; for each of `flypanel caddy`: `systemctl is-active --quiet $u && run systemctl stop $u`; `systemctl is-enabled --quiet $u && run systemctl disable $u`; remove `/etc/systemd/system/$u.service` when present (only if the unit file is ours: flypanel always, caddy only when its `ExecStart` mentions `/usr/local/bin/caddy`); `run systemctl daemon-reload`; `ok`.

- [ ] **Step 2: Implement `remove_caddy_config`**

`step step_caddy`; if `/etc/caddy/Caddyfile` missing → `say skip`; `caddyfile_strip /etc/caddy/Caddyfile`; on 1 also `remove_path /etc/caddy`; on 2 print a note that foreign config stays; `ok`.

- [ ] **Step 3: Implement `remove_files`**

`step step_files`; `remove_path "$PANEL_DIR"`, `/usr/local/bin/caddy`, `/var/lib/caddy`; `ok`.

- [ ] **Step 4: Implement `remove_users`**

`step step_user`; for `flypanel caddy`: `id -u $u >/dev/null 2>&1 || continue`; `run userdel "$u"`; if it fails and not `FORCE` → warn `err_user_busy` and set a warning flag instead of aborting; `ok` when at least one removed, `say skip` otherwise.

- [ ] **Step 5: Verify**

Run: `bash -n scripts/uninstall.sh && bash scripts/uninstall_test.sh && shellcheck scripts/uninstall.sh`
Expected: syntax ok, `0 failures`, shellcheck reports no errors (warnings reviewed).

- [ ] **Step 6: Commit**

```bash
git add scripts/uninstall.sh
git commit -m "feat(uninstall): remove units, files and users"
```

---

### Task 4: Firewall and SELinux rollback

**Files:**
- Modify: `scripts/uninstall.sh`
- Modify: `scripts/uninstall_test.sh`

**Interfaces:**
- Produces: `detect_family` (same logic as installer: `OS_RELEASE_FILE` override), `remove_firewall_rules` (skipped by `--skip-firewall`), `remove_selinux_ports`.

- [ ] **Step 1: Write the failing test**

Append a test that the SSH port is never in the removal list:

```bash
check "ssh port never removed" "! firewall_ports | grep -qx '22\|2222'"
check "web ports removed" "firewall_ports | grep -qx 80 && firewall_ports | grep -qx 443 && firewall_ports | grep -qx 8443"
```

- [ ] **Step 2: Run to verify it fails**

Run: `bash scripts/uninstall_test.sh`
Expected: FAIL — `firewall_ports: command not found`.

- [ ] **Step 3: Implement**

`firewall_ports()` prints `80 443 8443` one per line (never the SSH port, whatever it is). `remove_firewall_rules`: `step step_firewall`; `$SKIP_FIREWALL` → `say skip`; `have ufw && ufw status | grep -q active` → `run ufw --force delete allow <p>/tcp` per port; `have firewall-cmd && systemctl is-active --quiet firewalld` → `run firewall-cmd --permanent --remove-port=<p>/tcp` per port + `run firewall-cmd --reload`; never enable/disable the service; `ok`.
`remove_selinux_ports`: `step step_selinux`; `[ "$FAMILY" = dnf ]` and `getenforce` = `Enforcing` else `say skip`; `run semanage port -d -t http_port_t -p tcp 8443` and `... 3002`, tolerate failure (`|| true`).

- [ ] **Step 4: Verify**

Run: `bash scripts/uninstall_test.sh`
Expected: `0 failures`.

- [ ] **Step 5: Commit**

```bash
git add scripts/uninstall.sh scripts/uninstall_test.sh
git commit -m "feat(uninstall): roll back firewall and selinux ports"
```

---

### Task 5: main(), confirmation flow, logs, idempotency

**Files:**
- Modify: `scripts/uninstall.sh`

**Interfaces:**
- Produces: `parse_args`, `pick_lang`, `confirm_removal`, `summary`, `main`.

- [ ] **Step 1: Implement `parse_args` and `pick_lang`**

Flags `--lang --yes --dry-run --skip-firewall --force -h`; unknown flag → `usage` + exit 2. `pick_lang` copies installer behaviour (`LANG_SEL`, else `$LANG` prefix, else interactive `[1] ru [2] en [3] de`).

- [ ] **Step 2: Implement `confirm_removal`**

Print `warn_data`; `--yes` → allow; `--dry-run` → allow (nothing is deleted anyway); else `printf '%s' "$(t ask_delete)"` and `read -r ans`; `word_ok "$ans"` else `say wrong_word; say aborted; exit 1`.

- [ ] **Step 3: Implement `main`**

Order: `parse_args` → `pick_lang` → root check (before opening the log, comment: `# Root before the log is opened: an unprivileged run must not print errors.`) → `detect_family` when `/etc/os-release` readable (unknown family only warns unless `FORCE`) → log via `exec > >(tee -a "$UNINSTALL_LOG") 2>&1` → `welcome` → if nothing to remove (`! -e "$PANEL_DIR"` and no units and no caddy binary) → `say nothing; exit 0` → `confirm_removal` → steps in order `stop_units remove_caddy_config remove_files remove_users remove_firewall_rules remove_selinux_ports` → remove `/var/log/flypanel-install.log` → `summary` (list of removed + `keep_notes`) → `rm -f "$UNINSTALL_LOG"`.

- [ ] **Step 4: Verify end to end in dry-run**

Run: `bash scripts/uninstall.sh --dry-run --yes --lang en` (on the Mac: `PANEL_DIR` absent, so the "nothing to remove" path must print and exit 0).
Expected: exit 0, no files created, `[dry-run]` lines only where steps run.

- [ ] **Step 5: Commit**

```bash
git add scripts/uninstall.sh
git commit -m "feat(uninstall): main flow, confirmation and logs"
```

---

### Task 6: Docs, release asset, live verification

**Files:**
- Modify: `README.md`
- Modify: `.github/workflows/release.yml`

- [ ] **Step 1: Workflow**

In the "collect assets" step add `cp scripts/uninstall.sh bin/` and extend the checksum command to `sha256sum flypanel-linux-amd64 install.sh uninstall.sh > sha256sums.txt`; add `bin/uninstall.sh` to the release `files:` list.

- [ ] **Step 2: README**

New section `## Uninstall` after the install section: download `uninstall.sh` from releases/latest, `sudo bash uninstall.sh`, note that panel data is deleted and the word `delete` is required, list of what is kept (packages, firewall still enabled, SSH rule), `--dry-run`, `--skip-firewall`.

- [ ] **Step 3: Validate workflow YAML**

Run: `python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/release.yml'))"`
Expected: no output, exit 0.

- [ ] **Step 4: Live run on the VM**

Copy `scripts/uninstall.sh` to `home-fly`, run `sudo -n bash uninstall.sh --yes --lang ru`. Then assert: `id flypanel` fails, `[ ! -d /opt/flypanel ]`, `systemctl status flypanel` unknown, `ss -ltn` has no 3002/10811/8443, `/etc/caddy/Caddyfile` gone, `ufw status` still active without 80/443/8443, `https://devbydev.crazedns.ru` unreachable. Re-run the script → `nothing to remove`, exit 0.
Then reinstall from the published asset: `sudo -n bash install.sh --yes --lang ru --domain devbydev.crazedns.ru`, confirm all 13 steps pass, HTTPS 200, create admin, and record both results in the spec's verification section.

- [ ] **Step 5: Commit and release**

```bash
git add README.md .github/workflows/release.yml docs/superpowers/specs/2026-11-22-uninstaller-design.md
git commit -m "docs: uninstall section and release asset"
```

Then (explicit consent required) merge `dev` → `main`, push, tag `v0.1.3`, push tag, verify the release lists `uninstall.sh` and its checksum matches the local file.