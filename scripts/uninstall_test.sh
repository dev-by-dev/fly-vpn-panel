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

printf '%s failures\n' "$fails"
[ "$fails" -eq 0 ]