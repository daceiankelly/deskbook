# Backup, restore and disaster recovery

## What is backed up automatically

After **every** successful change the server writes a full copy to
`<data dir>/backups/deskbook-<UTC timestamp>-r<revision>.json` (atomically, like
the main file) and keeps the newest `backupRetention` copies (default 200,
editable under Admin → Settings). Admins can also click **Back up now**.

Bookings from previous days are deleted from the live file every time the
server starts (see architecture.md). Backups are the only place older history
survives, so keep off-server copies if you need records of past desk use.

These backups live on the same disk. For disaster recovery copy them off the server too.

## Off-server copies

Nightly copy to another machine or a removable drive, e.g. cron on the server:

```cron
# 02:15 every night: copy backups to a NAS mounted at /mnt/nas (read-only use only!)
15 2 * * * deskbook rsync -a --delete /var/lib/deskbook/backups/ /mnt/nas/deskbook-backups/
```

Copying is safe while the server runs: backups are written to a temp name and
renamed, so `rsync` never sees half a file. Also copy `deskbook.json` itself; a
plain `cp` of it is safe for the same reason. Never let anything other than
deskbook **write** into the data directory.

## Verify a file

```bash
/opt/deskbook/deskbook verify /var/lib/deskbook/backups/deskbook-20261028T091502.123Z-r00000042.json
# OK  schema=1 revision=42 updated=… desks=34 employees=25 bookings=310 (active 120)
```

`verify` performs the full strict validation the server performs on startup.

## Restore a specific backup

The server must be stopped: `restore` takes the same lock and refuses while it runs.

```bash
sudo systemctl stop deskbook
sudo -u deskbook /opt/deskbook/deskbook restore -data /var/lib/deskbook/deskbook.json \
     /var/lib/deskbook/backups/deskbook-…-r00000042.json
sudo systemctl start deskbook
```

`restore` validates the backup first (rejecting malformed JSON, unknown schema
versions and integrity errors), keeps the current file as
`deskbook.json.before-restore-<timestamp>`, then installs the backup atomically.

## Recover after corruption

If the server will not start and the journal shows
`data file is malformed` / `unexpected schema version` / `data failed validation`:

```bash
sudo systemctl stop deskbook                 # if it is restart-looping
sudo -u deskbook /opt/deskbook/deskbook recover -data /var/lib/deskbook/deskbook.json
sudo systemctl start deskbook
```

`recover` scans backups newest-first, skips any that fail validation, and
restores the newest valid one. The bad file is preserved alongside for inspection.

If the data file is **missing** but backups exist, the server refuses to start
(rather than silently creating an empty office). Run `recover` as above. A
brand-new data file is only created when there is neither a data file nor any backup.

## Lost admin password

```bash
sudo systemctl stop deskbook
sudo -u deskbook DESKBOOK_NEW_PASSWORD='new-long-password' \
     /opt/deskbook/deskbook set-password -data /var/lib/deskbook/deskbook.json admin
sudo systemctl start deskbook
```

## Full server loss

1. Install a new server (deployment.md §2–5) with the same LAN IP / DNS name.
2. Copy the newest off-server backup to `/var/lib/deskbook/restore.json`.
3. `deskbook verify restore.json`, then `deskbook restore -data /var/lib/deskbook/deskbook.json restore.json`.
4. `systemctl start deskbook`. Employees sign in again (sessions are not persisted);
   bookings made after the last off-server copy are lost, so tell staff to re-check.

## What the design guarantees

* A change is reported as successful only after the new file has been fsynced and
  renamed into place. A crash or I/O error before that leaves the previous file
  intact and the client receives HTTP 500 "Nothing was booked or changed".
* Partial temp files from an interrupted write are removed at the next start and
  never read.
