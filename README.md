# Musical Chairs — offline LAN desk booking

Musical Chairs is the name users see in the browser. Internally the code,
binary, container image and data file keep the name `deskbook`.

An internal desk-booking app for one office network. Each employee's browser runs
the React UI and its own Pyodide (Python-in-WebAssembly) instance for instant rule
feedback. A single Go server on the LAN serves the app, owns the bookings and
stores them in one versioned JSON file.

```
 Browser (React + Tailwind + SVG plan)          Office server (single instance)
 ┌──────────────────────────────────┐   HTTP   ┌───────────────────────────────────┐
 │ UI ─► Pyodide: deskbook.* rules  │ ───────► │ Go API: auth, CSRF, validation    │
 │      (feedback only)             │  /api/*  │   └─ store.Update (mutex):        │
 │ Static files + /pyodide/ runtime │ ◄─────── │       read latest → check → write │
 └──────────────────────────────────┘          │       tmp → fsync → rename        │
                                               │ data/deskbook.json  + backups/    │
                                               └───────────────────────────────────┘
```

## Layout

| Path | Contents |
|---|---|
| `server/` | Go 1.24 module, standard library only (no third-party deps) |
| `server/internal/store` | `Store` interface + JSON implementation, atomic writes, backups, restore, instance lock |
| `server/internal/booking` | Authoritative booking rules (window, overlap, hours, ownership) |
| `server/internal/auth` | PBKDF2-SHA256 password hashing, sessions, CSRF, login rate limit |
| `server/internal/api` | HTTP handlers |
| `server/cmd/deskbook` | Server binary + `verify` / `restore` / `recover` / `set-password` commands |
| `web/` | React 18 + Vite 5 + Tailwind 3 frontend |
| `web/src/python/deskbook` | Python rules: `intervals`, `policy`, `availability`, `validation`, `bridge` |
| `web/src/pyodide/` | JS ⇄ Pyodide interface ([README](web/src/pyodide/README.md)) |
| `deploy/` | systemd unit, build script, `podman/` (Quadlet unit, smoke test, Windows test script) |
| `Containerfile`, `.gitlab-ci.yml` | container image and GitLab pipeline ([guide](docs/podman-gitlab.md)) |
| `docs/` | [Podman & GitLab](docs/podman-gitlab.md), [architecture](docs/architecture.md), [deployment](docs/deployment.md), [backup & recovery](docs/backup-recovery.md), [testing](docs/testing.md), [design reference](docs/design-reference.md) |

## Quick start (build machine needs internet once; the office server never does)

Requirements: Go ≥ 1.24, Node ≥ 20, Python ≥ 3.10 (tests only).

```bash
./deploy/build.sh                       # tests + builds release/ (binary + web/dist)
cd release
DESKBOOK_ADMIN_PASSWORD='choose-a-long-one' ./deskbook -addr 0.0.0.0:8080
```

Open `http://<server-LAN-IP>:8080`, sign in as `admin`, then add employees and
adjust the floor plan under **Admin**.

## Important constraints

* **Run exactly one server process per data file.** Writers are serialised by an
  in-process mutex; an OS lock file stops a second process on the same host, but
  nothing coordinates servers on different machines. Never point two servers at
  the same file on a network share.
* The browser-side Python checks are convenience only. The Go API re-validates
  every create, change and cancel against the latest stored data.
* The data file must live outside `web/dist`; the server refuses to start otherwise
  and only ever serves files from the web directory.
