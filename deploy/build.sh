#!/usr/bin/env bash
# Builds a self-contained release/ folder: deskbook binary + web/dist.
# Run on a machine with internet access (for npm install) or a local npm mirror.
# The resulting folder needs no internet at runtime.
set -euo pipefail
cd "$(dirname "$0")/.."

GOOS="${GOOS:-linux}"
GOARCH="${GOARCH:-amd64}"

echo "== Python rule tests"
(cd web/src/python && python3 -m unittest discover -s tests)

echo "== Go tests"
(cd server && go vet ./... && go test -race -count=1 ./...)

echo "== Frontend build (copies Pyodide locally, checks offline)"
(cd web && { if [ -f package-lock.json ]; then npm ci; else npm install --no-audit --no-fund; fi; } && npm run build)

echo "== Server binary ($GOOS/$GOARCH)"
rm -rf release && mkdir -p release/web
(cd server && CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" go build -trimpath -ldflags="-s -w" -o ../release/deskbook$([ "$GOOS" = windows ] && echo .exe) ./cmd/deskbook)
cp -r web/dist release/web/dist
cp deploy/deskbook.service release/
echo "Release ready in ./release"
