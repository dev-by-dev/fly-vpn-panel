#!/usr/bin/env bash
# Unit checks for scripts/install.sh. Run: bash scripts/install_test.sh
set -uo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT
PASS=0
FAIL=0

export INSTALL_LIB_ONLY=1
export INSTALL_LOG=$WORK/install.log
# shellcheck source=install.sh
. "$ROOT/scripts/install.sh"
set +e   # install.sh turns on -e; the checks below expect non-zero results

check() { # check <name> <expected> <actual>
  if [ "$2" = "$3" ]; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
    printf 'FAIL %s: want [%s] got [%s]\n' "$1" "$2" "$3"
  fi
}

os_release() { # os_release <id> <version> [id_like]
  local f=$WORK/os-release
  printf 'ID=%s\nVERSION_ID=%s\nID_LIKE="%s"\nPRETTY_NAME="%s %s"\n' "$1" "$2" "${3:-}" "$1" "$2" > "$f"
  OS_RELEASE_FILE=$f
}

# --- detect_family -----------------------------------------------------------
os_release ubuntu 24.04 debian;      check "family ubuntu 24.04" apt "$(detect_family)"
os_release centos 9 "";              check "family centos 9"     dnf "$(detect_family)"
os_release centos 10 "";             check "family centos 10"    dnf "$(detect_family)"
os_release rocky 9 "rhel centos fedora"; check "family id_like rhel" dnf "$(detect_family)"
os_release debian 12 "";             check "family debian 12"    unsupported "$(detect_family)"
os_release arch "" "";               check "family arch"         unsupported "$(detect_family)"

# --- valid_domain ------------------------------------------------------------
valid_domain panel.fly-net.io;       check "domain ok"           0 $?
valid_domain example.com && R=0 || R=$?; check "domain example.com" 1 "$R"
valid_domain -bad.fly.io && R=0 || R=$?; check "domain leading dash" 1 "$R"
valid_domain no-tld && R=0 || R=$?;  check "domain no tld"        1 "$R"
valid_domain bad_domain.fly.io && R=0 || R=$?; check "domain underscore" 1 "$R"

# --- port_free_or_ours -------------------------------------------------------
LISTEN_PORT=45997
port_free_or_ours "$LISTEN_PORT"; check "port free" 0 $?
python3 -c "import socket,time; s=socket.socket(); s.bind(('127.0.0.1',$LISTEN_PORT)); s.listen(1); time.sleep(30)" &
LPID=$!
sleep 1
port_free_or_ours "$LISTEN_PORT" && R=0 || R=1; check "port busy" 1 "$R"
kill "$LPID" 2>/dev/null; wait "$LPID" 2>/dev/null

# --- detect_ssh_port ---------------------------------------------------------
printf '#!/bin/sh\necho "port 2222"\n' > "$WORK/sshd_t"; chmod +x "$WORK/sshd_t"
SSHD_T_CMD=$WORK/sshd_t; check "ssh port from sshd -T" 2222 "$(detect_ssh_port)"
printf 'Port 2200\n' > "$WORK/sshd_config"
SSHD_T_CMD=/bin/false SSHD_CONFIG=$WORK/sshd_config; check "ssh port from config" 2200 "$(detect_ssh_port)"
mkdir -p "$WORK/bin"; printf '#!/bin/sh\nexit 0\n' > "$WORK/bin/ss"; chmod +x "$WORK/bin/ss"
PATH="$WORK/bin:$PATH" SSHD_T_CMD=/bin/false SSHD_CONFIG=$WORK/nonexistent
check "ssh port unknown" "" "$(PATH="$WORK/bin:$PATH" detect_ssh_port)"

# --- firewall_allowed --------------------------------------------------------
SSH_PORT=22; firewall_allowed "$SSH_PORT"; check "firewall allowed with ssh port" 0 $?
SSH_PORT=""; firewall_allowed "$SSH_PORT" && R=0 || R=1; check "firewall blocked without ssh port" 1 "$R"

# --- env_file_write ----------------------------------------------------------
PANEL_DIR=$WORK/panel
IP=203.0.113.10
DOMAIN=panel.fly-net.io
mkdir -p "$PANEL_DIR"
env_file_write "$PANEL_DIR/.env"
check "env mode 600" 600 "$(stat -c %a "$PANEL_DIR/.env")"
grep -q '^PANEL_DOMAIN=panel.fly-net.io$' "$PANEL_DIR/.env"; check "env domain" 0 $?
grep -q '^LISTEN_ADDR=127.0.0.1:3002$' "$PANEL_DIR/.env"; check "env listen addr" 0 $?
env_file_write "$PANEL_DIR/.env"
BACKUPS=$(find "$PANEL_DIR" -name '.env.bak-*' | wc -l)
check "env backup created" 1 "$BACKUPS"

# --- caddyfile_apply ---------------------------------------------------------
CF=$WORK/Caddyfile
caddyfile_apply "$CF"; check "caddy empty file" 0 $?
grep -q 'metrics' "$CF"; check "caddy metrics added" 0 $?
grep -q '^# flypanel-managed begin$' "$CF"; check "caddy markers" 0 $?
grep -q "reverse_proxy 127.0.0.1:3002" "$CF"; check "caddy proxy" 0 $?

DOMAIN=other.fly-net.io
caddyfile_apply "$CF"; check "caddy replace managed" 0 $?
check "caddy old domain gone" 0 "$(grep -c 'panel.fly-net.io' "$CF" || true)"
check "caddy new domain" 1 "$(grep -c 'other.fly-net.io {' "$CF" || true)"
check "caddy markers single" 1 "$(grep -c '^# flypanel-managed begin$' "$CF" || true)"

FOREIGN=$WORK/Caddyfile.foreign
printf 'someone-else.io {\n\troot * /srv\n\tfile_server\n}\n' > "$FOREIGN"
cp "$FOREIGN" "$WORK/foreign.copy"
DOMAIN=panel.fly-net.io
caddyfile_apply "$FOREIGN" && R=0 || R=$?
check "caddy foreign site rejected" 1 "$R"
cmp -s "$FOREIGN" "$WORK/foreign.copy"; check "caddy foreign untouched" 0 $?

FOREIGN_GLOBAL=$WORK/Caddyfile.global
printf '{\n\tadmin off\n}\n\npanel.fly-net.io {\n\treverse_proxy 127.0.0.1:9\n}\n' > "$FOREIGN_GLOBAL"
cp "$FOREIGN_GLOBAL" "$WORK/global.copy"
caddyfile_apply "$FOREIGN_GLOBAL" && R=0 || R=$?
check "caddy foreign global rejected" 1 "$R"
cmp -s "$FOREIGN_GLOBAL" "$WORK/global.copy"; check "caddy foreign global untouched" 0 $?

KEEP_GLOBAL=$WORK/Caddyfile.keep
printf '{\n\tmetrics\n\temail admin@fly-net.io\n}\n\n# our own note\n' > "$KEEP_GLOBAL"
caddyfile_apply "$KEEP_GLOBAL"; check "caddy keeps existing global" 0 $?
grep -q 'email admin@fly-net.io' "$KEEP_GLOBAL"; check "caddy global preserved" 0 $?
check "caddy metrics not duplicated" 1 "$(grep -c 'metrics' "$KEEP_GLOBAL" || true)"

# --- dry-run end to end ------------------------------------------------------
OS_RELEASE_FILE=$WORK/os-release
os_release ubuntu 24.04 debian
INSTALL_LIB_ONLY=0 bash "$ROOT/scripts/install.sh" --dry-run --yes --skip-dns --lang en \
  --domain panel.fly-net.io --ip 203.0.113.10 > "$WORK/dry.out" 2>&1
check "dry-run exit 0" 0 $?
grep -q 'flypanel installer' "$WORK/dry.out"; check "dry-run speaks english" 0 $?
grep -q 'dry-run' "$WORK/dry.out"; check "dry-run prints actions" 0 $?

printf '\n%d passed, %d failed\n' "$PASS" "$FAIL"
[ "$FAIL" -eq 0 ]