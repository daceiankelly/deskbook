# Deskbook container image: Go API + built React/Pyodide frontend.
#
#   podman build -t deskbook .
#   podman run -d --name deskbook -p 8080:8080 -v deskbook-data:/data \
#     -e DESKBOOK_ADMIN_PASSWORD='first-start-only' deskbook
#
# Run ONE container per data volume. The image needs no internet at runtime.

ARG NODE_IMAGE=docker.io/library/node:20.18.0-bookworm-slim
ARG GO_IMAGE=docker.io/library/golang:1.24-bookworm
ARG RUNTIME_IMAGE=gcr.io/distroless/static-debian12:nonroot

# ---- frontend: React + Tailwind + locally bundled Pyodide ----
FROM ${NODE_IMAGE} AS web
WORKDIR /src/web
# scripts/ is needed by the postinstall hook that copies Pyodide into public/.
COPY web/package*.json ./
COPY web/scripts ./scripts
RUN if [ -f package-lock.json ]; then npm ci; else npm install --no-audit --no-fund; fi
COPY web/ ./
# build = copy Pyodide runtime + vite build + static offline check (fails the build on external URLs)
RUN npm run build

# ---- backend: static Go binary ----
FROM ${GO_IMAGE} AS server
WORKDIR /src/server
COPY server/go.mod ./
COPY server/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/deskbook ./cmd/deskbook \
 && mkdir -p /out/data

# ---- runtime: no shell, no package manager, non-root (uid 65532) ----
FROM ${RUNTIME_IMAGE}
WORKDIR /app
COPY --from=server /out/deskbook /app/deskbook
COPY --from=web /src/web/dist /app/web/dist
# A new named volume mounted on /data inherits this ownership.
COPY --from=server --chown=65532:65532 /out/data /data
USER 65532:65532
VOLUME ["/data"]
EXPOSE 8080
STOPSIGNAL SIGTERM
ENTRYPOINT ["/app/deskbook"]
CMD ["serve", "-addr", "0.0.0.0:8080", "-data", "/data/deskbook.json", "-web", "/app/web/dist"]
