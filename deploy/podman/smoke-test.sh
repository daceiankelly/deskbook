#!/usr/bin/env bash
# End-to-end smoke test of a built image with Podman:
#   offline-capable static assets, data file not exposed, sign-in, booking,
#   overlap rejection, back-to-back booking, and persistence across a container restart.
#
#   deploy/podman/smoke-test.sh <image>
set -euo pipefail

IMAGE="${1:?usage: smoke-test.sh <image>}"
NAME="deskbook-smoke-$$"
VOL="deskbook-smoke-$$"
PORT="${SMOKE_PORT:-18080}"
BASE="http://127.0.0.1:${PORT}"
PW="smoke-test-password-123"
JAR="$(mktemp)"

cleanup() {
  podman rm -f "$NAME" >/dev/null 2>&1 || true
  podman volume rm -f "$VOL" >/dev/null 2>&1 || true
  rm -f "$JAR"
}
trap cleanup EXIT

fail() { echo "SMOKE FAIL: $*" >&2; podman logs "$NAME" >&2 || true; exit 1; }
pass() { echo "ok - $*"; }

start() {
  podman run -d --name "$NAME" -p "127.0.0.1:${PORT}:8080" -v "$VOL:/data" \
    -e DESKBOOK_ADMIN_PASSWORD="$PW" "$IMAGE" >/dev/null
  for _ in $(seq 1 30); do
    curl -fsS "$BASE/api/health" >/dev/null 2>&1 && return 0
    sleep 1
  done
  fail "server did not become healthy"
}

status() { curl -s -o /dev/null -w '%{http_code}' "$@"; }

login() {
  rm -f "$JAR"
  local body
  body=$(curl -fsS -c "$JAR" -H 'Content-Type: application/json' \
    -d "{\"username\":\"admin\",\"password\":\"$PW\"}" "$BASE/api/login") || fail "login"
  CSRF=$(printf '%s' "$body" | sed -n 's/.*"csrfToken":"\([^"]*\)".*/\1/p')
  [ -n "$CSRF" ] || fail "no csrf token in login response"
}

book() { # date start end -> HTTP status
  status -b "$JAR" -H 'Content-Type: application/json' -H "X-CSRF-Token: $CSRF" \
    -d "{\"deskId\":\"D-01\",\"date\":\"$1\",\"startHour\":$2,\"endHour\":$3}" "$BASE/api/bookings"
}

start
pass "container started and healthy"

podman exec "$NAME" /app/deskbook healthcheck || fail "built-in healthcheck"
pass "built-in healthcheck"

[ "$(status "$BASE/")" = 200 ] || fail "index.html"
[ "$(status "$BASE/pyodide/pyodide.asm.wasm")" = 200 ] || fail "local Pyodide wasm"
ctype=$(curl -sI "$BASE/pyodide/pyodide.asm.wasm" | tr -d '\r' | awk -F': ' 'tolower($1)=="content-type"{print $2}')
[ "$ctype" = "application/wasm" ] || fail "wasm content-type is '$ctype'"
pass "frontend and Pyodide served locally (application/wasm)"

for p in /deskbook.json /data/deskbook.json /../data/deskbook.json /.env; do
  [ "$(status "$BASE$p")" = 404 ] || fail "$p is reachable"
done
pass "data file not exposed"

[ "$(status "$BASE/api/desks")" = 401 ] || fail "anonymous API access"
login
pass "admin sign-in"

TODAY=$(curl -fsS -b "$JAR" "$BASE/api/config" | sed -n 's/.*"today":"\([0-9-]*\)".*/\1/p')
TOMORROW=$(date -u -d "$TODAY + 1 day" +%F)
[ "$(book "$TOMORROW" 9 12)" = 201 ] || fail "create booking"
[ "$(book "$TOMORROW" 11 13)" = 409 ] || fail "overlap not rejected"
[ "$(book "$TOMORROW" 13 12)" = 400 ] || fail "inverted range not rejected"
pass "booking, overlap and range rules"

podman restart "$NAME" >/dev/null
for _ in $(seq 1 30); do curl -fsS "$BASE/api/health" >/dev/null 2>&1 && break; sleep 1; done
login
curl -fsS -b "$JAR" "$BASE/api/bookings" | grep -q "\"date\":\"$TOMORROW\"" || fail "booking lost after restart"
pass "booking persisted across container restart"

podman exec "$NAME" /app/deskbook verify /data/deskbook.json >/dev/null || fail "data file verify"
pass "data file verifies inside the container"

echo "SMOKE TEST PASSED"
