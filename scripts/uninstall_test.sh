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

printf '%s failures\n' "$fails"
[ "$fails" -eq 0 ]