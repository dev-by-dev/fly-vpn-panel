# Audit Must-Fix 1–6 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the six Must-fix findings from the 2026-11-22 audit: xray restart race, non-reality inbound killing subscriptions, three install.sh defects, and the uninstaller deleting a foreign Caddy unit.

**Architecture:** Six independent fixes, each with its own regression test and commit. Go fixes touch `internal/xray` (op lock serializing public lifecycle ops) and `internal/sub` (skip unparseable inbounds instead of failing the render). Shell fixes are surgical: delete one premature restart, fail fast instead of spinning, create the log 0600, and gate Caddy-unit removal on our binary path.

**Tech Stack:** Go 1.27 stdlib (`os/exec`, `sync`), existing bash test harnesses (`scripts/install_test.sh`, `scripts/uninstall_test.sh`).

**Spec:** `docs/superpowers/specs/2026-11-22-audit-must-fix-design.md`

## Global Constraints

- No new dependencies, no new files outside test additions to existing test files.
- Public API of `sub` and `xray` packages unchanged (no signature changes outside `internal/xray/proc.go` internals).
- `supervise` keeps using only `m.mu`; it must never take the new op lock (deadlock).
- All shell changes must keep `INSTALL_LIB_ONLY` / `UNINSTALL_LIB_ONLY` sourcing guards working (macOS bash 3.2 must still source both scripts without executing main).
- Test harnesses are Linux-oriented (`stat -c`); `bash -n` and `go test` must pass on macOS, harness runs happen on Linux.
- Commit messages: English, conventional commits, one per task.

## Review Focus

Five failure modes the spec implies but a naive implementation could miss:

1. **Concurrent `Start` (not only `Restart`) in the race test** — a lock covering only `Restart` still lets `Start` race `Stop`. Pinned by Task 1: the goroutine mix includes `Start` calls (`i%4 == 3`).
2. **All inbounds unparseable must yield empty output, not an error** — pinned by Task 2: single `inboundNoReality()` fixture renders `"[]"` with `err == nil`.
3. **Supervisor respawning a child while `Stop` runs** — state must not be cleared for a stale handle. Pinned by Task 1: `stopLocked` clears `running`/`cmd` only `if m.cmd == cmd`; existing `TestManagerCrashBackoff` plus the race test are the regression net.
4. **Pre-existing world-readable log from an older install** — `install -m 600 /dev/null` recreates (truncates) the file each run, fixing mode on upgrade. Pinned by Task 5: the test asserts mode `600` on a fresh call.
5. **Unreadable Caddy unit (non-root dry-run) must count as foreign, never "ours"** — pinned by Task 6: chmod-000 unit → `caddy_unit_is_ours` returns 1.

Not testable headless: interactive tty re-ask on invalid domain (Task 4 keeps that path by construction — `! $ASSUME_YES && [ -t 0 ]` loops).

---

### Task 1: Serialize xray lifecycle ops

**Files:**
- Modify: `internal/xray/proc.go:25-124`
- Test: `internal/xray/proc_test.go`

**Interfaces:**
- Consumes: existing `Manager`, `supervise`, `backoffFor`, test helpers `fakeXray`, `processGone`, `assertNoLeftovers`.
- Produces: `Manager` gains field `opMu sync.Mutex`; unexported `startLocked() error`, `stopLocked() error`. Public `Start/Stop/Restart` signatures unchanged.

- [ ] **Step 1: Write the failing test**

Add `TestManagerConcurrentRestart` to `internal/xray/proc_test.go`, following the existing helper style:

```go
func TestManagerConcurrentRestart(t *testing.T) {
	bin := fakeXray(t, "exec sleep 86400\n")
	m := NewManager(bin, cfgPath(t))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	const n = 8
	errCh := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%4 == 3 {
				errCh <- m.Start() // no-op, must not race
			} else {
				errCh <- m.Restart()
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent op: %v", err)
		}
	}
	st := m.Status()
	if !st.Running || st.PID <= 0 {
		t.Fatalf("not running after concurrent restarts: %+v", st)
	}
	pid := st.PID
	if err := m.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if m.Status().Running {
		t.Fatal("still running after stop")
	}
	processGone(t, pid)
	assertNoLeftovers(t, bin)
}
```

If `cfgPath(t)` does not exist under that name, reuse however `TestManagerStartStop` builds the manager and config path — copy its setup lines verbatim.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -timeout 30s -run TestManagerConcurrentRestart ./internal/xray/ -v`
Expected: FAIL — hang until the 30s timeout kills the run (data race / interleaved Stop+Start), or an immediate assertion failure. Either counts as red.

- [ ] **Step 3: Implement the op lock in `internal/xray/proc.go`**

Add to the `Manager` struct (after `mu`):

```go
	opMu sync.Mutex // serializes public Start/Stop/Restart; mu guards fields
```

Rename bodies: `Start()` becomes `func (m *Manager) Start() error { m.opMu.Lock(); defer m.opMu.Unlock(); return m.startLocked() }` where `startLocked` is the current Start body verbatim. Same shape for `Stop()` → `stopLocked`, with one change in the final state-clearing section: kill the current child if running (as today), but clear `running = false; m.cmd = nil` only when `m.cmd == cmd` (the handle captured at entry) — a supervisor respawn may have swapped `m.cmd` mid-stop. `Restart()` becomes:

```go
func (m *Manager) Restart() error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	if err := m.stopLocked(); err != nil {
		return err
	}
	return m.startLocked()
}
```

Leave `supervise` untouched (it holds only `m.mu` and respawns inline; `startLocked`'s no-op guard would refuse crash respawns, so the supervisor must not call it — and it must never take `opMu` or it deadlocks against `Stop`).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -timeout 60s ./internal/xray/ -v`
Expected: PASS, including existing `TestManagerStartStop` and `TestManagerCrashBackoff`.

- [ ] **Step 5: Commit**

```bash
git add internal/xray/proc.go internal/xray/proc_test.go
git commit -m "fix(xray): serialize start/stop/restart with an op lock"
```

### Task 2: Skip unparseable inbounds in JSON and YAML renders

**Files:**
- Modify: `internal/sub/json.go:19`, `internal/sub/yaml.go:28-38`
- Test: `internal/sub/sub_test.go`

**Interfaces:**
- Consumes: `inboundParams`, existing fixtures `inboundA`, `testUser`, `testEntryIP`, `DefaultTemplate`, `newTestServer`, store APIs.
- Produces: new test fixture `inboundNoReality() store.Inbound`.

- [ ] **Step 1: Write the failing test**

Add fixture and test to `internal/sub/sub_test.go` (copy `testInbound`'s map-building style; verify against `internal/sub/links.go` that a `streamSettings` with `security: "none"` and no `realitySettings` makes `inboundParams` return an error):

```go
func inboundNoReality() store.Inbound {
	return testInbound("in-noreality", 8445, "", "", "", "", "", "")
}
```

If passing empty strings through `testInbound` does not produce a config missing `realitySettings`, build the map inline instead: `port: 8445, tag: "in-noreality", streamSettings: {network: "tcp", security: "none"}` — the point is `inboundParams(inboundNoReality())` returns an error.

```go
func TestRenderSkipsUnparseableInbound(t *testing.T) {
	user, entry := testUser(), testEntryIP

	out, err := RenderJSON([]store.Inbound{inboundNoReality(), inboundA()}, user, entry, DefaultTemplate("json"))
	if err != nil {
		t.Fatalf("render json: %v", err)
	}
	if strings.Contains(out, "in-noreality") || !strings.Contains(out, `"in-a"`) {
		t.Fatalf("json output wrong: %s", out)
	}

	params, err := proxyParams([]store.Inbound{inboundNoReality(), inboundA()}, user, entry)
	if err != nil {
		t.Fatalf("proxy params: %v", err)
	}
	if strings.Contains(params[PROXIES], "in-noreality") || !strings.Contains(params[PROXIES], "in-a") {
		t.Fatalf("yaml proxies wrong: %s", params[PROXIES])
	}

	empty, err := RenderJSON([]store.Inbound{inboundNoReality()}, user, entry, DefaultTemplate("json"))
	if err != nil || empty != "[]" {
		t.Fatalf("all-unparseable: out=%q err=%v", empty, err)
	}
}
```

Use whatever import name the file already has for `strings` (add if missing). Check the actual key constant for proxies (`PROXIES` or literal) against `TestRenderYAML` and match it. Then add the handler-level case modeled on `TestSubAccess` (see `sub_test.go` ~L429–460 for exact user fields and routing): `newTestServer`, create a user with `SubToken`, `st.InboundCreate(&inboundNoReality())` plus `st.InboundCreate(&inboundA())`, `GET /sub/{token}/json` must return 200.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestRenderSkipsUnparseableInbound ./internal/sub/ -v`
Expected: FAIL — `RenderJSON` returns the `missing reality settings` error.

- [ ] **Step 3: Change both render loops**

In `internal/sub/json.go` and in `proxyParams` in `internal/sub/yaml.go`, replace `return nil, err` after `inboundParams` with `continue`, each with the comment:

```go
		// One odd inbound must not 500 the whole subscription; RenderBase64 skips it the same way.
```

No other changes; the existing empty fallbacks already render `"[]"` / empty proxies when nothing valid remains.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/sub/ -v`
Expected: PASS, including `TestRenderJSONArray`, `TestRenderYAML`, `TestSubAccess`.

- [ ] **Step 5: Commit**

```bash
git add internal/sub/json.go internal/sub/yaml.go internal/sub/sub_test.go
git commit -m "fix(sub): skip unparseable inbounds in json and yaml renders"
```

### Task 3: Drop premature caddy restart from SELinux step

**Files:**
- Modify: `scripts/install.sh:710` (`configure_selinux`)
- Test: `scripts/install_test.sh`

**Interfaces:**
- Consumes: existing `configure_selinux`, `run`, `have`; harness `check`, PATH-stub pattern.
- Produces: none.

- [ ] **Step 1: Write the failing test**

In `scripts/install_test.sh`, add a section using the existing stub pattern (see the apt-get stub section for the shape):

```bash
# configure_selinux must not restart units (caddy.service does not exist yet
# at that point; the restart aborts the install under set -e).
sl=$WORK/selstub; sel_log=$WORK/sel.log; : >"$sel_log"
mkdir -p "$sl"
printf '#!/bin/sh\necho Enforcing\n' >"$sl/getenforce"
printf '#!/bin/sh\necho "$@" >>"%s"\n' "$sel_log" >"$sl/semanage"
printf '#!/bin/sh\necho "$@" >>"%s"\n' "$sel_log" >"$sl/systemctl"
chmod +x "$sl"/*
( export PATH="$sl:$PATH" FAMILY=dnf DRY_RUN=false ACCESS_PORT=8443 SELINUX_ENFORCING=1
  configure_selinux >/dev/null 2>&1 )
check "selinux step issues no restart" 0 "$(grep -c restart "$sel_log")"
check "selinux step labels both ports" 2 "$(grep -c http_port_t "$sel_log")"
```

Note: `configure_selinux` calls `getenforce` only when `have getenforce` succeeds — the stub satisfies that. If the function reads other variables the section must export them; read `configure_selinux` (install.sh ~L701–712) first and match its actual inputs. `DRY_RUN=false` because `run` short-circuits in dry-run.

- [ ] **Step 2: Run test to verify it fails**

Run (Linux/VM with bash 5): `bash scripts/install_test.sh`
Expected: FAIL — `selinux step issues no restart` expected 0, got 1.

- [ ] **Step 3: Delete the restart line**

Remove `run systemctl restart caddy flypanel` from `configure_selinux` in `scripts/install.sh`. Both services still get restarted where their units are created (`install_unit`, `install_caddy`). Leave the trailing `ok` call.

- [ ] **Step 4: Run tests to verify they pass**

Run: `bash scripts/install_test.sh`
Expected: all pass (`0 failed`), including the dry-run e2e at the end.

- [ ] **Step 5: Commit**

```bash
git add scripts/install.sh scripts/install_test.sh
git commit -m "fix(install): drop premature caddy restart from selinux step"
```

### Task 4: Fail fast on invalid domain with --yes or non-tty stdin

**Files:**
- Modify: `scripts/install.sh:452-460` (`collect_answers`)
- Test: `scripts/install_test.sh`

**Interfaces:**
- Consumes: `ask`, `valid_domain`, `fatal`, `t q_domain`.
- Produces: helper `collect_answers_timeout <seconds>` in the test harness (returns the function's exit code, or 124 on hang).

- [ ] **Step 1: Write the failing test**

In `scripts/install_test.sh`:

```bash
# collect_answers must die (not spin forever) when it cannot re-ask:
# --yes returns the empty default instantly and non-tty stdin cannot prompt.
collect_answers_timeout() {
	( collect_answers </dev/null >/dev/null 2>&1 ) &
	local pid=$1 i
	pid=$!
	for i in $(seq 1 $(( $2 * 10 ))); do kill -0 "$pid" 2>/dev/null || break; sleep 0.1; done
	if kill -0 "$pid" 2>/dev/null; then kill -9 "$pid"; wait "$pid" 2>/dev/null; return 124; fi
	wait "$pid"; return $?
}

( export ASSUME_YES=true DOMAIN="" EMAIL=admin@example.com; collect_answers_timeout x 3 )
check "yes-mode empty domain dies" 1 "$?"
( export ASSUME_YES=true DOMAIN="bad_domain.fly-io" EMAIL=admin@example.com; collect_answers_timeout x 3 )
check "yes-mode invalid domain dies" 1 "$?"
( export ASSUME_YES=false DOMAIN="bad_domain.fly-io" EMAIL=admin@example.com; collect_answers_timeout x 3 )
check "non-tty invalid domain dies" 1 "$?"
ASSUME_YES=false DOMAIN=""
```

If `collect_answers` needs more inputs (fw choice, dns_points_here does network), pre-export whatever variables it reads so it reaches the domain check — read the function first and export only what is needed to avoid network calls. The subshell `( ... )` wrapper keeps the harness clean; drop the stray `local pid=$1` line if it offends — the point is: background, poll `kill -0` up to N seconds, 124 on survival.

- [ ] **Step 2: Run test to verify it fails**

Run: `bash scripts/install_test.sh`
Expected: FAIL — first case reports 124 (the loop is spinning) instead of 1.

- [ ] **Step 3: Rework the domain loop**

Replace the domain block at the top of `collect_answers` with:

```bash
	while true; do
		[ -n "$DOMAIN" ] || DOMAIN=$(ask "$(t q_domain)" "")
		valid_domain "$DOMAIN" && break
		# --yes or piped stdin cannot re-ask: die instead of spinning.
		if $ASSUME_YES || [ ! -t 0 ]; then
			fatal "invalid domain: $DOMAIN"
		fi
		DOMAIN=""
	done
```

Delete the now-unreachable post-loop `if ! valid_domain "$DOMAIN"; then fatal ...; fi`. Interactive tty keeps re-asking (per spec).

- [ ] **Step 4: Run tests to verify they pass**

Run: `bash scripts/install_test.sh`
Expected: all pass. Dry-run e2e unaffected (it passes a valid `--domain`).

- [ ] **Step 5: Commit**

```bash
git add scripts/install.sh scripts/install_test.sh
git commit -m "fix(install): fail fast on invalid domain with --yes or non-tty"
```

### Task 5: Create the install log 0600

**Files:**
- Modify: `scripts/install.sh` (new `open_install_log`; `main` ~L919-921)
- Test: `scripts/install_test.sh`

**Interfaces:**
- Consumes: `INSTALL_LOG` (env-overridable, harness sets it), `DRY_RUN`.
- Produces: `open_install_log()` — creates/truncates `$INSTALL_LOG` mode 600 and redirects stdout+stderr through `tee -a`.

- [ ] **Step 1: Write the failing test**

In `scripts/install_test.sh`:

```bash
# summary() prints the admin password; umask 022 alone leaves the log 0644.
( INSTALL_LOG="$WORK/open-log.log" open_install_log ) >/dev/null 2>&1
check "install log created 600" 600 "$(stat -c %a "$WORK/open-log.log" 2>/dev/null || echo missing)"
```

The subshell protects the harness's own fds; `tee` exits when the subshell closes.

- [ ] **Step 2: Run test to verify it fails**

Run: `bash scripts/install_test.sh`
Expected: FAIL — `open_install_log: command not found` path yields `missing`.

- [ ] **Step 3: Implement `open_install_log`**

In `scripts/install.sh` (near the other output helpers):

```bash
open_install_log() {
	# summary() prints the admin password; umask alone gives the log 0644.
	install -m 600 /dev/null "$INSTALL_LOG"
	exec > >(tee -a "$INSTALL_LOG") 2>&1
}
```

In `main`, replace `exec > >(tee -a "$INSTALL_LOG") 2>&1` inside `if ! $DRY_RUN; then ...; fi` with a single call: `open_install_log`. (Recreating the file each run also fixes the mode left by older installs.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `bash scripts/install_test.sh`
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add scripts/install.sh scripts/install_test.sh
git commit -m "fix(install): create the install log 0600"
```

### Task 6: Only remove a Caddy unit that runs our binary

**Files:**
- Modify: `scripts/uninstall.sh:206-213` (`caddy_unit_is_ours`), var block near L414
- Test: `scripts/uninstall_test.sh`

**Interfaces:**
- Consumes: existing `check <name> <eval-expr>` harness, `DRY_RUN`, `PANEL_UNIT` precedent.
- Produces: env-overridable `CADDY_UNIT` (default `/etc/systemd/system/caddy.service`).

- [ ] **Step 1: Write the failing test**

In `scripts/uninstall_test.sh`, using the eval-based `check`:

```bash
# caddy_unit_is_ours must match only a unit running /usr/local/bin/caddy;
# anything else — foreign path, unreadable, missing — is not ours.
CADDY_UNIT=$tmp/caddy.service
printf '[Service]\nExecStart=/usr/local/bin/caddy run --config /etc/caddy/Caddyfile\n' >"$CADDY_UNIT"
check "our unit is ours" "caddy_unit_is_ours"
printf '[Service]\nExecStart=/usr/bin/caddy run\n' >"$CADDY_UNIT"
check "foreign unit is not ours" "! caddy_unit_is_ours"
if [ "$(id -u)" -ne 0 ]; then
	chmod 000 "$CADDY_UNIT"
	check "unreadable unit is not ours" "! caddy_unit_is_ours"
	chmod 644 "$CADDY_UNIT"
fi
CADDY_UNIT=$tmp/gone.service
check "missing unit is not ours" "! caddy_unit_is_ours"
CADDY_UNIT=/etc/systemd/system/caddy.service
```

- [ ] **Step 2: Run test to verify it fails**

Run (bash 5): `bash scripts/uninstall_test.sh`
Expected: FAIL — `foreign unit is not ours`: current code treats any readable unit as ours.

- [ ] **Step 3: Rewrite `caddy_unit_is_ours`**

In `scripts/uninstall.sh`, add next to `PANEL_UNIT` (keep the same block/style):

```bash
CADDY_UNIT=${CADDY_UNIT:-/etc/systemd/system/caddy.service}
```

Replace the function body with:

```bash
caddy_unit_is_ours() {
	[ -f "$CADDY_UNIT" ] || return 1
	# Anything else — foreign path, unreadable file — is not ours; never remove it.
	grep -q '/usr/local/bin/caddy' "$CADDY_UNIT" 2>/dev/null
}
```

`DRY_RUN` no longer relaxes the check (unreadable → grep fails → foreign; conservative by spec). `stop_units` stays as-is (out of scope).

- [ ] **Step 4: Run tests to verify they pass**

Run: `bash scripts/uninstall_test.sh`
Expected: `0 failures`.

- [ ] **Step 5: Commit**

```bash
git add scripts/uninstall.sh scripts/uninstall_test.sh
git commit -m "fix(uninstall): only remove a caddy unit that runs our binary"
```

---

## Final verification (after all tasks)

```bash
go test ./...
bash -n scripts/install.sh scripts/uninstall.sh
shellcheck scripts/install.sh scripts/uninstall.sh   # if available
bash scripts/install_test.sh                          # Linux/bash5
bash scripts/uninstall_test.sh                        # Linux/bash5
```

On macOS only `go test` and `bash -n` are reliable (harnesses use `stat -c`); run the two harness scripts on the Linux VM.
