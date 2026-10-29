# Test on Windows, build in GitLab, run with Podman

```
Windows PC (Podman Desktop)        GitLab CI                         Office server (Podman)
deploy/podman/test-windows.ps1 ──► test:python  test:go  test:web ─► podman pull <registry>/deskbook:<sha>
 same images as CI                 image:podman: build → smoke → push   Quadlet unit, volume deskbook-data:/data
```

The UI is the React app (JavaScript) with the Python rules running as
WebAssembly through Pyodide. Both are static files baked into the image under
`/app/web/dist` and served by the Go binary. The browser needs nothing from the internet.

## 1. On Windows

1. Install Podman Desktop and start a machine (it runs on WSL 2):
   ```powershell
   winget install RedHat.Podman-Desktop
   podman machine init
   podman machine start
   ```
2. Run everything CI runs:
   ```powershell
   powershell -ExecutionPolicy Bypass -File deploy\podman\test-windows.ps1
   ```
   This runs the Python tests, `go vet` + Go tests with the race detector, builds
   the image (the frontend build and offline check happen inside), then runs
   `deploy/podman/smoke-test.sh` inside the Podman machine against a real
   container: health, local Pyodide `.wasm` served as `application/wasm`, data
   file not reachable, sign-in, booking/overlap/range rules, and persistence
   across `podman restart`. The smoke step assumes the default rootless machine;
   if you switched it to rootful (`podman machine set --rootful`), run
   `podman machine ssh "sudo bash /mnt/c/…/deploy/podman/smoke-test.sh deskbook:local"` instead.
3. Try the app: `podman run --rm -p 8080:8080 -e DESKBOOK_ADMIN_PASSWORD=change-me-now-please deskbook:local`
   and open http://localhost:8080.

Native alternative without containers: install Go ≥ 1.24, Node ≥ 20 and Python
(`winget install GoLang.Go OpenJS.NodeJS.LTS Python.Python.3.12`), then
`cd server; go test ./...`, `cd web; npm install; npm run dev` (Vite proxies
`/api` to `go run ./cmd/deskbook -data ..\data\deskbook.json` on :8080).

After the first successful `npm install`, commit `web/package-lock.json` so CI
and the image use `npm ci` with identical dependency versions.

## 2. GitLab CI (`.gitlab-ci.yml`)

| Job | Image | Does |
|---|---|---|
| `test:python` | python:3.12-slim | rule tests |
| `test:go` | golang:1.24 | vet + race tests |
| `test:web` | node:20 | `npm run build` incl. offline check; `web/dist` artifact |
| `image:podman` | quay.io/podman/stable | build `Containerfile`, smoke test, push `:<short sha>`; also `:latest` on the default branch |

Runner requirements for `image:podman`:

* **Docker executor:** set `privileged = true` for that runner in `config.toml`
  (nested Podman needs it). `STORAGE_DRIVER=vfs` and `BUILDAH_ISOLATION=chroot`
  are already set in the job.
* **Shell executor on a Linux host with Podman:** delete the job's `image:` line.
* **No privileged runners allowed:** replace the job with Kaniko to build/push
  and run the smoke test in a later deployment environment instead.

The registry login uses GitLab's built-in `CI_REGISTRY_*` variables; nothing to configure.
CI needs internet (or mirrors) for base images, npm and Go; the resulting image does not.

## 3. On the office server

```bash
podman login registry.gitlab.example.com              # deploy token with read_registry
printf '%s' 'a-long-admin-password' | podman secret create deskbook-admin-password -
sudo cp deploy/podman/deskbook.container /etc/containers/systemd/   # edit Image= and PublishPort= first
sudo systemctl daemon-reload
sudo systemctl start deskbook
sudo systemctl status deskbook
```

Quadlet generates a systemd service, so the container starts on boot and restarts
on failure; the built-in `deskbook healthcheck` kills it if `/api/health` stops
answering. Firewall rules from [deployment.md](deployment.md#4-firewall) still apply.

**Upgrade:** change `Image=` to the new commit tag (or keep `:latest`), then
`podman pull <image> && sudo systemctl restart deskbook`. Avoid
`AutoUpdate=registry` unless you want unattended restarts during office hours.

### Single instance

Run exactly one Deskbook container per `deskbook-data` volume. Do not use
`--replicas`, a Kubernetes Deployment with more than one replica, or a volume
shared over the network. The OS lock inside the volume refuses a second container
on the same host, but nothing coordinates separate hosts.

### Data, backups, recovery in a container

The volume holds `deskbook.json`, the lock file and `backups/`. Commands from
[backup-recovery.md](backup-recovery.md) run through the image:

```bash
# copy backups off the server (safe while running)
podman volume export deskbook-data > deskbook-data-$(date +%F).tar

# verify the live file
podman exec deskbook /app/deskbook verify /data/deskbook.json

# recover / restore: stop first, then run the command in a throw-away container
sudo systemctl stop deskbook
podman run --rm -v deskbook-data:/data <image> recover -data /data/deskbook.json
podman run --rm -v deskbook-data:/data <image> restore -data /data/deskbook.json /data/backups/deskbook-…json
sudo systemctl start deskbook

# reset a password
sudo systemctl stop deskbook
podman run --rm -e DESKBOOK_NEW_PASSWORD='new-long-password' -v deskbook-data:/data <image> \
  set-password -data /data/deskbook.json admin
sudo systemctl start deskbook
```

To move to a new server: `podman volume export` on the old one,
`podman volume create deskbook-data && podman volume import deskbook-data file.tar` on the new one.
