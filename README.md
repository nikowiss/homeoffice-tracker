# Homeoffice Tracker

A personal web app to record home office days and count them per year for the tax report.

## Start

```sh
podman compose up -d --build
```

Open <http://localhost:40000>. The app is available only on this computer.

## Data

The app keeps all data in one SQLite file: `data/homeoffice.db`. The file stays when you stop, rebuild, or remove the container. `podman compose down -v` does not delete it.

## Backup

Copy `data/homeoffice.db` to a safe location. For a consistent copy, stop the app first:

```sh
podman compose stop
cp data/homeoffice.db ~/Backups/homeoffice-$(date +%F).db
podman compose start
```

## After a reboot

The container starts again only if the Podman machine starts. Set the Podman machine to start at macOS login (Podman Desktop setting), or run `podman machine start`.

## Tests

```sh
go test ./...
```
