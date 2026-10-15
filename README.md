# Homeoffice Tracker

A personal web app to mark home office days and count them per year for the tax report.

## Start

```sh
podman compose up -d
```

Podman pulls the released image `ghcr.io/nikowiss/homeoffice-tracker:1`. Open <http://localhost:40000>. The app is available only on this computer.

## Update

```sh
podman compose pull
podman compose up -d
```

The tag `:1` gets all `1.x` releases. A new major version needs a change of the tag in `compose.yaml`.

## Data

The app keeps all data in one SQLite file: `data/homeoffice.db`. The file stays when you stop, rebuild, or remove the container. `podman compose down -v` does not delete it.

## Backup

Copy `data/homeoffice.db` to a safe location. For a consistent copy, stop the app first:

```sh
podman compose stop
cp data/homeoffice.db ~/Backups/homeoffice-$(date +%F).db
podman compose start
```

## Reminders

On weekdays at 09:00, the browser shows a reminder to mark today. The reminder does not come on Saturday or Sunday, or if today is already marked.

1. Click "Enable reminders" below the year counts, and allow notifications.
2. In macOS System Settings → Notifications → Arc, allow notifications.

Arc must run at the reminder time. An open window is not necessary. A Focus mode can hide the reminder. If the Mac sleeps at 09:00, the reminder comes later on the same day.

To change the time, set `REMINDER_TIME` (`HH:MM`) in `compose.yaml`.

To test the reminder, run:

```sh
curl -X POST http://localhost:40000/reminder/test
```

The container needs internet access to send a reminder.

## After a reboot

The container starts again only if the Podman machine starts. Set the Podman machine to start at macOS login (Podman Desktop setting), or run `podman machine start`.

## Tests

```sh
go test ./...
```

## Local image

```sh
podman build -f Containerfile --build-arg VERSION=dev -t homeoffice-tracker:dev .
```

## Release

1. Merge PRs to `main`. The PR title must use a conventional prefix, e.g. `feat:`, `fix:`, `chore:`. A `!` after the type, e.g. `feat!:`, marks a breaking change.
2. Release Drafter updates the draft release on GitHub. The prefixes set the categories and the next version: breaking change → major, `feat` → minor, all other prefixes → patch.
3. Review the draft and click "Publish release".
4. The release workflow runs the tests. It then pushes the image for `linux/amd64` and `linux/arm64` to `ghcr.io/nikowiss/homeoffice-tracker` with the tags `X.Y.Z`, `X.Y`, `X` and `latest`.

To override the calculated version, run the "Release Drafter" workflow manually with the input `version`.
