# Audit Must-fix 1–6 — design

Source: `fly-vpn-panel — аудит 2026-11-22` (Obsidian), findings 1–6, all
re-verified against the code at `e0d63c4` before writing this spec.

## Why

The audit found six defects that break core guarantees: one corrupted
xray process under concurrent restarts, one broken subscription for
everyone when a single inbound is odd, two installer failures, one
leaked admin password, and one uninstaller that deletes a foreign
Caddy. Should/Nice findings (7–20) are a separate later campaign.

## Fixes

### 1. Concurrent xray restart corrupts the manager — `internal/xray/proc.go`

`Restart() = Stop() + Start()` with no operation-level lock. Two
concurrent restarts interleave: R1's kill kills R2's child, R2's
supervisor starts a third process, R1 hangs forever in `wg.Wait()`.

Fix:

- add `opMu sync.Mutex` to `Manager`;
- `Start`, `Stop`, `Restart` take `opMu` for the whole operation;
- internals become unexported `startLocked`/`stopLocked` so `Restart`
  runs as one serialized stop+start;
- the crash-restart path inside `supervise` keeps using only `m.mu`
  (it must not deadlock on `opMu`); it calls `startLocked`;
- `stopLocked` clears `running`/`cmd` only when `m.cmd == cmd` (the
  cmd it captured), so a supervisor-restarted child is never mistaken
  for the one being stopped.

Test (`proc_test.go`): N goroutines calling `Restart()` concurrently
on a fake xray; afterwards `Status().Running` is true, no goroutine
hangs (bounded by test timeout), and repeated Stop leaves no zombie.

### 2. One inbound without Reality breaks all subscriptions — `internal/sub`

`inboundParams` errors on `missing reality settings`; `RenderJSON`
(json.go) and `proxyParams` (yaml.go) turn that into a 500 for
`/sub/{token}/json|mihomo|clash` for **all** users. `RenderBase64`
already skips such inbounds.

Fix: both loops `continue` past an `inboundParams` error, exactly like
`RenderBase64`. If *every* enabled inbound is unparseable, the
subscription renders empty — same as base64 today, consistent and
non-fatal.

Test (`sub_test.go`): enabled TCP inbound without realitySettings +
one valid reality inbound → RenderJSON/RenderYAML succeed, output
contains only the valid inbound; request path returns 200.

### 3. CentOS install dies mid-run — `scripts/install.sh:710`

`configure_selinux()` ends with `systemctl restart caddy flypanel`,
but `caddy.service` is created later in `install_caddy`. With
`set -e` the run aborts after the firewall step.

Fix: delete that line. Restarts already happen in `install_caddy` and
`install_unit`.

### 4. `--yes` with a bad domain loops forever — `scripts/install.sh:452`

`ask` under `ASSUME_YES` (or non-tty stdin) returns the empty default
instantly; the `while [ -z "$DOMAIN" ]` loop spins on CPU forever.

Fix: after `valid_domain` fails, if `$ASSUME_YES || [ ! -t 0 ]` →
`fatal invalid domain` instead of retrying. Interactive stdin keeps
re-asking.

Test (`install_test.sh`): `collect_answers` with `ASSUME_YES=true`,
invalid `DOMAIN`, non-tty → exits non-zero, does not hang.

### 5. Admin password in a world-readable log — `scripts/install.sh`

`exec > >(tee -a "$INSTALL_LOG")` creates the log with umask 022
(0644); `summary()` prints the generated password into it.

Fix: `install -m 600 /dev/null "$INSTALL_LOG"` immediately before the
`exec` (inside the `! $DRY_RUN` branch).

Test (`install_test.sh`): the log-creating step in a temp dir →
`stat` shows mode 600.

### 6. Uninstaller deletes a foreign Caddy — `scripts/uninstall.sh:208`

`caddy_unit_is_ours` falls through to `[ -r "$unit" ] || $DRY_RUN`;
any readable unit file is judged "ours" and removed together with
`/var/lib/caddy` and the `caddy` user.

Fix: the unit is ours iff it references `/usr/local/bin/caddy`:
`grep -q … && return 0; return 1`. An unreadable unit (non-root
dry-run) now reports "not ours" — conservative, safe: real runs are
root anyway. Comment updated.

Test (`uninstall_test.sh`): unit with our path → 0; readable unit
with another path → 1; no unit file → 1.

## Non-goals

- findings 7–20 and the minor cleanup list (separate campaign);
- changing the installer's step order or adding new steps;
- release/CI changes (finding 16).

## Verification

1. `go test ./...` (new tests 1–2 included).
2. `bash -n` + shellcheck on both scripts; `bash scripts/install_test.sh`;
   `bash scripts/uninstall_test.sh` (all findings 3–6 tests).
3. Manual on the next deploy: CentOS path and a real `--yes` install
   are covered by the existing VM ritual in
   `fly-vpn-panel — релизы и деплой` (not blocking this batch).
