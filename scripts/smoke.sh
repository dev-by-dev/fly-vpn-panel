#!/usr/bin/env bash
# Local smoke test: full build (SPA embedded), admin init, serve on
# 127.0.0.1:3002, login -> cookie -> GET /api/users -> GET / serves the
# built SPA. No real identifiers: loopback only, throwaway data dir,
# stub xray binary (real xray is not required locally; the panel keeps
# serving HTTP when xray is unavailable).
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

BASE_URL="http://127.0.0.1:3002"
PASSWORD="test-password-123"

fail() { echo "FAIL: $*" >&2; exit 1; }

TMP="$(mktemp -d)"
PANEL_PID=""
cleanup() {
  [ -n "$PANEL_PID" ] && kill "$PANEL_PID" 2>/dev/null || true
  rm -rf "$TMP"
}
trap cleanup EXIT

echo "== build (npm ci + vite build + go build with embedded SPA) =="
make build

echo "== temp env + xray stub =="
mkdir -p "$TMP/data"
cat > "$TMP/xray-stub.sh" <<'EOF'
#!/bin/sh
# shortcut: Manager.Start probes "xray version" and waits for its output;
# a bare sleep would block that probe for an hour. Answer it, then park.
if [ "$1" = "version" ]; then echo "xray stub"; exit 0; fi
sleep 3600
EOF
chmod +x "$TMP/xray-stub.sh"

cat > "$TMP/env" <<EOF
DATA_DIR=$TMP/data
LISTEN_ADDR=127.0.0.1:3002
XRAY_BIN=$TMP/xray-stub.sh
POLL_INTERVAL_SEC=30
COOKIE_SECURE=false
EOF

echo "== admin init (password via stdin) =="
printf '%s\n' "$PASSWORD" | FLYPANEL_ENV="$TMP/env" ./bin/flypanel admin init --password-stdin admin

echo "== serve =="
FLYPANEL_ENV="$TMP/env" ./bin/flypanel serve >"$TMP/server.log" 2>&1 &
PANEL_PID=$!

for _ in $(seq 1 50); do
  if curl -s -o /dev/null "$BASE_URL/"; then break; fi
  sleep 0.2
done
curl -s -o /dev/null "$BASE_URL/" || { cat "$TMP/server.log" >&2; fail "server did not come up on $BASE_URL"; }

echo "== login -> cookie jar =="
CODE="$(curl -s -o /dev/null -w '%{http_code}' -c "$TMP/jar" \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"admin\",\"password\":\"$PASSWORD\"}" \
  "$BASE_URL/api/auth/login")"
[ "$CODE" = "200" ] || fail "login: expected 200, got $CODE"
echo "login: 200"

echo "== GET /api/users (authenticated) =="
CODE="$(curl -s -o /dev/null -w '%{http_code}' -b "$TMP/jar" "$BASE_URL/api/users")"
[ "$CODE" = "200" ] || fail "GET /api/users: expected 200, got $CODE"
echo "GET /api/users: 200"

echo "== GET / serves the built SPA =="
curl -s -b "$TMP/jar" "$BASE_URL/" -o "$TMP/index.html"
grep -q '/assets/index-' "$TMP/index.html" || fail "index.html has no hashed asset ref (/assets/index-*)"
grep -q 'build me' "$TMP/index.html" && fail "index.html is still the placeholder" || true
echo "GET /: built SPA with hashed assets"

echo "PASS"
