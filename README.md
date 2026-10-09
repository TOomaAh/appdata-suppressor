# appdata-suppressor

A small web UI that lists the folders of an `appdata` directory (Unraid, etc.),
shows which ones are no longer mounted by any Docker container, and lets you
move them to a trash, restore them, or delete them permanently.

A folder counts as **used** when at least one container (running or stopped) has a
bind mount on that folder or on one of its subfolders.

The web interface is in French.

Scans run in the background: one starts when the server boots, and the latest
result is cached, so the page loads instantly. The folder list and container
usage show up right away; sizes, the slow part on a large appdata, fill in as
they are computed. Moving folders to the trash updates the cached result without
a full rescan.

## Build

```sh
CGO_ENABLED=0 GOOS=linux go build -o appdata-suppressor .
```

The front end (`web/static`) is embedded in the binary.

## Usage

```sh
APPDATA_PASSWORD='a-strong-password' ./appdata-suppressor -path /mnt/cache/appdata -alias /mnt/user/appdata
```

Then open http://server:8765 and log in as `admin` with the password you chose.

| Flag           | Default             | Description                                                      |
|----------------|---------------------|------------------------------------------------------------------|
| `-path`        | `/mnt/nvme/appdata` | appdata directory to scan                                        |
| `-alias`       | —                   | Another path pointing to `-path` (e.g. `/mnt/user/appdata`), repeatable |
| `-host`        | all interfaces      | Interface to listen on                                           |
| `-port`        | `8765`              | Port to listen on                                                |
| `-user`        | `admin`             | Login user name (or `APPDATA_USER` env var)                      |
| `-password`    | —                   | Login password; prefer the `APPDATA_PASSWORD` env var            |
| `-session-ttl` | `168h`              | How long a login stays valid                                     |
| `-docker-host` | `unix:///var/run/docker.sock` | Docker daemon, `unix://` or `tcp://` (or `DOCKER_HOST` env var) |
| `-trash-days`  | `7`                 | Days before trashed folders are permanently deleted (`0` = never) |
| `-no-auth`     | `false`             | Disable authentication                                           |

The binary needs access to the Docker socket and permission to delete inside
`-path`. It talks to the Docker Engine API directly; the `docker` CLI is not needed.
TLS-protected TCP daemons and Windows named pipes are not supported.

## Trash

Deleting a folder moves it to `<path>/.appdata-trash/`. It stays on the same
filesystem, so the move is instant and frees no space yet. From the **Corbeille**
view you can restore a folder to its original name or delete it permanently.

Trashed folders are permanently deleted after `-trash-days` days (checked at
startup and then hourly). Entries are named `<deletion time>_<original name>`; a
folder dropped into the trash by hand is never purged automatically.

## Docker

A multi-arch image (`linux/amd64`, `linux/arm64`) is built by GitHub Actions and
published to `ghcr.io/toomaah/appdata-suppressor`:

| Tag        | Built from                     |
|------------|--------------------------------|
| `latest`   | the default branch             |
| `1.2.3`, `1.2` | a `v1.2.3` git tag        |
| `sha-…`    | every pushed commit            |

To build locally instead:

```sh
docker compose up -d --build
```

Edit [compose.yaml](compose.yaml) first: password, paths, aliases. To use the
published image, replace `build: .` with
`image: ghcr.io/toomaah/appdata-suppressor:latest`.

- The appdata folder must be mounted **at the same path** as on the host. Docker
  reports mount sources as host paths: with a different path inside the container,
  no folder would be recognized as used.
- The `/var/run/docker.sock` socket is required to list containers. It grants
  root-equivalent access to the host (mounting it `:ro` does not change that),
  which is one more reason to keep authentication enabled.

Without compose:

```sh
docker build -t appdata-suppressor .
docker run -d --name appdata-suppressor -p 8765:8765 \
  -e APPDATA_PASSWORD='a-strong-password' \
  -v /mnt/cache/appdata:/mnt/cache/appdata \
  -v /var/run/docker.sock:/var/run/docker.sock \
  appdata-suppressor -path /mnt/cache/appdata -alias /mnt/user/appdata
```

## Authentication

The server refuses to start without a password, unless `-no-auth` is set (only use
it when a reverse proxy already handles authentication).

- Password: pass it through `APPDATA_PASSWORD` rather than `-password`, since
  command-line arguments are visible in `ps`.
- Sessions: `HttpOnly`, `SameSite=Strict` cookie, kept in memory. Restarting the
  server logs everyone out.
- After 5 failed logins, the client IP is locked out for 1 minute.
- ⚠️ Over plain HTTP, the password travels unencrypted. For access from outside
  your local network, put the tool behind an HTTPS reverse proxy. The cookie
  becomes `Secure` automatically when the proxy sends `X-Forwarded-Proto: https`.

## Layout

```
main.go                  flags, server startup and graceful shutdown
internal/auth            login, sessions, brute-force protection
internal/docker          lists containers and their bind mounts (Engine API)
internal/scanner         folder scan, background scan job, sizes, trash
internal/server          HTTP API (/api/scan, /api/delete, /api/trash/...)
web/static               user interface (HTML/CSS/JS)
```

## Tests

```sh
go test ./...
```
