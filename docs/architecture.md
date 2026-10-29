# Architecture

## Data file (`data/deskbook.json`)

```json
{
  "schemaVersion": 1,
  "revision": 42,
  "updatedAt": "2026-10-28T09:15:02Z",
  "settings":  { "officeName": "Main Office", "floorName": "Floor 1", "timezone": "Europe/Amsterdam",
                 "windowDays": 60, "openHour": 7, "closeHour": 19, "backupRetention": 200 },
  "desks":     [ { "id": "D-01", "label": "A1", "zone": "A", "x": 430, "y": 420, "rotation": 0, "active": true } ],
  "employees": [ { "id": "e-admin", "username": "admin", "name": "Administrator", "initials": "AD",
                   "role": "admin", "passwordHash": "pbkdf2-sha256$600000$…$…", "active": true, "createdAt": "…" } ],
  "bookings":  [ { "id": "b-…", "deskId": "D-01", "employeeId": "e-…", "date": "2026-10-29",
                   "startHour": 9, "endHour": 12, "status": "active", "createdAt": "…" } ]
}
```

* `date` + `startHour`/`endHour` are **office-local** (the configured IANA timezone),
  so a booking never shifts when a PC or the server is in another zone or DST changes.
* Intervals are half-open `[start, end)`: 09–12 and 12–14 do not overlap.
* Loading is strict: unknown fields, trailing data, missing/different
  `schemaVersion`, broken references, duplicate IDs or overlapping active
  bookings all reject the file. The server then refuses to start rather than
  overwrite it (see [backup-recovery.md](backup-recovery.md)).
* Cancelled bookings are kept (status `cancelled`) until the next server start.
* **Startup cleanup:** each time the server starts it deletes every booking
  (active or cancelled) dated before today in office time, in one normal atomic
  write. Today's bookings, including ones that already ended, and all future
  bookings are kept. If nothing is old, the file is not rewritten. The removed
  bookings still exist in the backups taken before that start, so "My bookings"
  shows past bookings only for the current day.

## Write path (`store.JSONStore.Update`)

All in one critical section guarded by a `sync.Mutex`:

1. Re-read and fully validate the file on disk (latest data; refuses to write if it is unreadable).
2. Run the handler's closure: booking rules are checked **against that latest copy**, then mutated.
3. Increment `revision`, validate the whole document again.
4. Write JSON to `.deskbook-*.tmp` **in the same directory**, `fsync`, close.
5. `rename` over `deskbook.json` (atomic on POSIX and NTFS), then `fsync` the directory (POSIX).
6. Only now replace the in-memory copy and return success. Any failure in 1–5
   returns an error → HTTP 500 "nothing was booked"; memory and file stay unchanged.
7. Write a timestamped backup copy (also atomically) and prune to `backupRetention`.

Because the check and the write happen under the same lock, two simultaneous
requests for the same desk are serialised: the second one sees the first booking and gets 409.

Temp files left by a crash between steps 4 and 5 are deleted at startup; the
original file is still the last committed state.

## Single instance

The mutex only coordinates goroutines inside one process. At startup the store
takes an OS lock on `deskbook.json.lock` (`flock` on Linux/macOS, exclusive
`CreateFile` on Windows). A second `deskbook serve`, or `restore`/`recover`/`set-password`
while the server runs, fails with "locked by another deskbook instance". The OS
releases the lock if the process dies, so a crash does not block restart.
This lock is **not** reliable on network filesystems: keep the data on local disk.

## Storage abstraction

Handlers depend only on the `store.Store` interface
(`Snapshot`, `Update(fn)`, `Backup`, `ListBackups`, `Close`). A database-backed
implementation can replace `JSONStore` by running `fn` inside a serialisable
transaction, without changing the API package.

## Authentication & security

* Passwords: PBKDF2-HMAC-SHA256, 600k iterations, 16-byte salt (Go standard library `crypto/pbkdf2`).
  Unknown usernames still run a hash to avoid timing leaks. 5 failures per username+IP in 15 min → 429.
* Sessions: 256-bit random token in an `HttpOnly`, `SameSite=Strict` cookie
  (`Secure` when TLS is enabled). Only its SHA-256 is kept server-side (memory).
  12 h sliding expiry. Server restart signs everyone out (bookings unaffected).
* CSRF: per-session token returned at sign-in, required in `X-CSRF-Token` on every
  non-GET request, plus an `Origin` same-host check. JSON-only bodies with unknown
  fields rejected, so `employeeId`/`role` can never be supplied by a client.
* Identity and role are loaded from the data file on every request, so
  deactivation or demotion takes effect immediately; deactivation also ends sessions
  and cancels the person's upcoming bookings.
* Static serving is confined to `web/dist`, dotfiles refused, and the server
  refuses to start if the data file is inside the web root.
* CSP `default-src 'self'; connect-src 'self'` — the browser itself blocks any
  request leaving the office server. `script-src 'self' 'wasm-unsafe-eval'`:
  WebAssembly compilation is allowed for Pyodide, JavaScript `eval`/`new Function`
  is not (verified with the bundled Pyodide 0.26.4 in Chromium). If an older
  browser fails to load the rules engine, `-allow-unsafe-eval` adds `'unsafe-eval'`.

## API

| Method & path | Who | Purpose |
|---|---|---|
| `POST /api/login`, `POST /api/logout` | any / signed-in | session |
| `GET /api/session` | anyone | `{user, csrfToken}`, or `{user: null}` when signed out (always 200) |
| `GET /api/config` | signed-in | settings + office `today`, `lastDate`, `nowHour` |
| `GET /api/desks`, `GET /api/colleagues` | signed-in | reference data |
| `GET /api/bookings?from&to` | signed-in | active bookings in range (with names, for the map & colleague search) |
| `GET /api/bookings/mine` | signed-in | own bookings incl. cancelled |
| `POST /api/bookings` | signed-in | `{deskId,date,startHour,endHour}` → 201 / 400 / 409 / 422 / 500 |
| `PATCH /api/bookings/{id}` | owner or admin | change (re-validated, not after start) |
| `POST /api/bookings/{id}/cancel` | owner or admin | cancel (not after end) |
| `GET/POST /api/admin/employees`, `PATCH /api/admin/employees/{id}` | admin | accounts |
| `POST /api/admin/desks`, `PATCH/DELETE /api/admin/desks/{id}` | admin | desks & plan positions |
| `PUT /api/admin/settings` | admin | settings |
| `GET/POST /api/admin/backups` | admin | list / create backups |
| `GET /api/health` | anyone | liveness + revision |

## Booking rules (same in Go and Python)

* Date within `today … today + windowDays − 1` in office time (60 days incl. today).
* `0 ≤ start < end ≤ 24`, whole hours, inside opening hours; today's start ≥ current office hour.
* Desk exists and is active.
* No overlap with another active booking on the same desk; back-to-back allowed.
* An employee cannot hold two desks at overlapping times.
* Change: owner/admin, booking active and not yet started, re-validated ignoring itself.
* Cancel: owner/admin, booking active and not yet ended.
