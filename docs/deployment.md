# Deployment on the office LAN

The server is one static binary plus the `web/dist` folder. It needs no internet,
no database and no Node.js at runtime. Examples use Linux with systemd; a Windows
alternative is at the end.

## 1. Build (once, on a machine with internet)

```bash
./deploy/build.sh                 # GOOS/GOARCH env vars select the target, default linux/amd64
```

This runs all tests, installs npm packages, copies the Pyodide runtime into
`web/public/pyodide`, builds `web/dist`, runs the offline check, and produces
`release/`. Copy `release/` to the server (USB stick, scp, etc.).

## 2. Install

```bash
sudo useradd --system --home /var/lib/deskbook --shell /usr/sbin/nologin deskbook
sudo mkdir -p /opt/deskbook /var/lib/deskbook
sudo cp -r release/deskbook release/web /opt/deskbook/
sudo chown -R root:root /opt/deskbook
sudo chown deskbook:deskbook /var/lib/deskbook && sudo chmod 700 /var/lib/deskbook
```

The data directory (`/var/lib/deskbook`) holds `deskbook.json`, its `.lock`, and
`backups/`. Keep it on a **local disk**, never on a network share.

## 3. LAN address

1. Give the server a fixed address: a DHCP reservation on the office router, or a
   static IP (e.g. `192.168.1.20/24`) via netplan / NetworkManager.
2. Bind to that interface with `-addr 192.168.1.20:8080` (edit `ExecStart` in
   `deskbook.service`). `0.0.0.0:8080` listens on all interfaces; prefer the LAN
   IP if the machine has other networks.
3. Optional friendly name: add `deskbook.office.lan → 192.168.1.20` on the office
   DNS/router, so staff open `http://deskbook.office.lan:8080`.
4. To use port 80 without root: `sudo setcap cap_net_bind_service=+ep /opt/deskbook/deskbook`
   and `-addr 192.168.1.20:80`.

### HTTPS (recommended)

With an internal CA or a self-signed certificate distributed to office PCs:

```
-tls-cert /etc/deskbook/cert.pem -tls-key /etc/deskbook/key.pem
```

TLS also turns on the `Secure` cookie flag. Without TLS, session cookies cross
the LAN unencrypted; that is only acceptable on a trusted, switched office network.

## 4. Firewall

Allow the port from the office subnet only.

ufw:
```bash
sudo ufw allow from 192.168.1.0/24 to any port 8080 proto tcp
sudo ufw enable
```

firewalld:
```bash
sudo firewall-cmd --permanent --new-zone=office
sudo firewall-cmd --permanent --zone=office --add-source=192.168.1.0/24
sudo firewall-cmd --permanent --zone=office --add-port=8080/tcp
sudo firewall-cmd --reload
```

No outbound rules are needed; the server never connects out.

## 5. Automatic startup

```bash
sudo cp release/deskbook.service /etc/systemd/system/   # edit the LAN IP in ExecStart first
sudo systemctl daemon-reload
sudo systemctl enable --now deskbook
journalctl -u deskbook -f
```

On first start with no data file the server creates one with the default floor
plan and an `admin` account. The password is taken from `DESKBOOK_ADMIN_PASSWORD`
if set (add `Environment=DESKBOOK_ADMIN_PASSWORD=…` to the unit for the first
start only, then remove it), otherwise a random one is printed once to the
journal. Change it after signing in (Admin → Employees → Reset password).

`Restart=on-failure` restarts after crashes; the OS lock is released when the
process dies, so a restart is never blocked. **Do not** run a second instance
for "high availability": only one process may own the data file.

## 6. Upgrades

```bash
sudo systemctl stop deskbook
sudo cp release/deskbook /opt/deskbook/deskbook && sudo rm -rf /opt/deskbook/web && sudo cp -r release/web /opt/deskbook/
sudo systemctl start deskbook
```

A build that expects a different `schemaVersion` refuses to start on an old
file instead of misreading it; a migration step would be shipped with such a release.

## Windows server alternative

Build with `GOOS=windows ./deploy/build.sh`. Place files in `C:\deskbook`, data in
`C:\deskbook-data` (outside the web folder). Allow the port:
`netsh advfirewall firewall add rule name="Deskbook" dir=in action=allow protocol=TCP localport=8080 remoteip=192.168.1.0/24`.
For automatic start, create a Task Scheduler task "At system startup", running as
a dedicated local account, action
`C:\deskbook\deskbook.exe serve -addr 192.168.1.20:8080 -data C:\deskbook-data\deskbook.json -web C:\deskbook\web\dist`,
with "Restart on failure" enabled and "Stop the task if it runs longer than" disabled.
