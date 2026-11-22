#!/usr/bin/env bash
# Unit tests for scripts/uninstall.sh. Runs on bash 5 (associative arrays).
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

tmp=$(mktemp -d)

# 1: only our global block and our site -> file goes away
mkdir -p "$tmp/a"; printf '{\n\tmetrics\n}\n' > "$tmp/a/Caddyfile"
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

# 4: dry-run rewrites nothing
mkdir -p "$tmp/d"; printf 'example.org {\n\trespond hi\n}\n\n# flypanel-managed begin\npanel.example.org {\n\tlog\n}\n# flypanel-managed end\n' > "$tmp/d/Caddyfile"
DRY_RUN=true
caddyfile_strip "$tmp/d/Caddyfile" >/dev/null 2>&1; rc=$?
DRY_RUN=false
check "dry-run keeps managed block" "[ $rc -eq 0 ] && grep -q flypanel-managed '$tmp/d/Caddyfile'"
check "dry-run keeps caddyfile" "caddyfile_has_content '$tmp/d/Caddyfile'"
rm -rf "$tmp"

check "ssh port never removed" "! firewall_ports | grep -qx '22\\|2222'"
check "web ports listed" "firewall_ports | grep -qx 80 && firewall_ports | grep -qx 443 && firewall_ports | grep -qx 8443"
# sshd on a web port: that rule must survive too
SSHD_T_CMD='echo port 443'
check "detected ssh port is filtered out" "! firewall_ports | grep -qx 443 && firewall_ports | grep -qx 80 && firewall_ports | grep -qx 8443"
SSHD_T_CMD='sshd -T'

# 6: begin marker without end -> foreign config after it must survive
mkdir -p "$tmp/f"; printf '{\n\tmetrics\n}\n\n# flypanel-managed begin\npanel.example.org {\n\tlog\n}\nexample.org {\n\trespond hi\n}\n' > "$tmp/f/Caddyfile"
caddyfile_strip "$tmp/f/Caddyfile" >/dev/null 2>&1; rc=$?
check "unclosed marker -> untouched (rc 2)" "[ $rc -eq 2 ] && [ -f '$tmp/f/Caddyfile' ] && grep -q 'example.org' '$tmp/f/Caddyfile'"

# quiet commands stay visible as [dry-run] lines
DRY_RUN=true
out=$(quiet false 2>&1); rc=$?
DRY_RUN=false
check "dry-run shows quiet commands" "[ $rc -eq 0 ] && printf '%s' \"\$out\" | grep -q 'dry-run'"
check "quiet runs for real" "quiet true"

# firewall steps belong to the panel, not to a stray caddy
PANEL_DIR=$tmp/none PANEL_UNIT=$tmp/none.service
check "no panel -> not installed" "! panel_installed"
mkdir -p "$tmp/g"; : > "$tmp/g/unit.service"
PANEL_DIR=$tmp/none PANEL_UNIT=$tmp/g/unit.service
check "panel unit counts as installed" "panel_installed"
PANEL_DIR=/opt/flypanel PANEL_UNIT=/etc/systemd/system/flypanel.service

# 5: our file and our backups -> directory is gone
mkdir -p "$tmp/e"; printf '{\n\tmetrics\n}\n# flypanel-managed begin\np.example.org {\n\tlog\n}\n# flypanel-managed end\n' > "$tmp/e/Caddyfile"
: > "$tmp/e/Caddyfile.bak-202601010000"
OLD_DIR=$CADDY_DIR; CADDY_DIR=$tmp/e
remove_caddy_config >/dev/null 2>&1
CADDY_DIR=$OLD_DIR
check "backups removed with our caddyfile" "[ ! -d '$tmp/e' ]"

# 7: packages the installer added are removed, nothing else
mkdir -p "$tmp/pk"; printf 'unzip\nufw\n' > "$tmp/pk/installed-packages"
PKG_FILE=$tmp/pk/installed-packages
check "package list is read" "[ \"\$(read_packages)\" = 'unzip ufw' ]"
PKG_FILE=$tmp/pk/missing
check "no list -> extras fallback" "[ \"\$(read_packages)\" = 'ufw unzip' ]"

mkdir -p "$tmp/stub"
cat > "$tmp/stub/apt-get" <<'SH'
#!/bin/sh
printf '%s\n' "$*" >> "$APT_LOG"
SH
chmod +x "$tmp/stub/apt-get"
APT_LOG=$tmp/apt.log; : > "$APT_LOG"; export APT_LOG
OLD_PATH=$PATH
FAMILY=apt; PKGS="unzip ufw"; PATH="$tmp/stub:$PATH"
remove_packages >/dev/null 2>&1
check "packages purged" "grep -qx 'purge -y -qq unzip ufw' '$APT_LOG'"
: > "$APT_LOG"; PKGS=""
remove_packages >/dev/null 2>&1
check "empty list purges nothing" "[ ! -s '$APT_LOG' ]"
PKGS="unzip"; KEEP_PACKAGES=true
remove_packages >/dev/null 2>&1
check "keep-packages purges nothing" "[ ! -s '$APT_LOG' ]"
KEEP_PACKAGES=false; PKGS=""
PATH=$OLD_PATH
rm -rf "$tmp/pk" "$tmp/stub" "$APT_LOG"

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
rm -f "$tmp/caddy.service"

printf '%s failures\n' "$fails"
[ "$fails" -eq 0 ]