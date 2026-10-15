# Spec: Homeoffice Tracker

Status: ready-for-agent

## Goal

A personal web app on `http://localhost:40000`. The owner clicks one button to mark **Today** as a **Home office day**. The owner can also mark **Past days** and unmark wrong days. The app shows the **Year count** for every year and gives one CSV **Export** per year. The app runs in the owner's local Podman. The data survives container restarts, image rebuilds, and redeploys.

Use the terms from [CONTEXT.md](../../CONTEXT.md).

## Users and access

- One user (the owner). No login.
- Bind the port to `127.0.0.1` only. Other devices on the network cannot reach the app.

## Domain rules

1. A **Home office day** is one calendar date. A date is marked or not marked. There are no duplicates, hours, or half days.
2. **Today** is the current date in time zone `Europe/Berlin`. The server is the only source of Today.
3. The owner cannot mark a date after Today. The server rejects such a request with `400`.
4. Weekends and public holidays are allowed. The app shows no warning for them.
5. There is no lower limit for years.
6. Marking an already marked date does nothing (idempotent). Unmarking a date that is not marked does nothing.
7. A **Year count** is the number of Home office days from 1 January to 31 December of one year.

## Stack

| Concern | Decision |
|---|---|
| Language | Go (current stable release) |
| HTTP | Standard library `net/http`, with method and path patterns in `ServeMux` (Go 1.22+) |
| HTML | `html/template`, server-rendered. Embed templates and CSS with `embed`. No frontend build step. JavaScript is not necessary. Plain HTML forms do all writes. |
| Storage | SQLite through `modernc.org/sqlite` (pure Go, no CGO). This allows a static binary. |
| Time zone data | `import _ "time/tzdata"`, so the minimal image does not need system zoneinfo |

## Data

One SQLite file at `/data/homeoffice.db` in the container.

```sql
CREATE TABLE IF NOT EXISTS home_office_days (
  date TEXT PRIMARY KEY  -- ISO 8601, e.g. 2026-10-14
);
```

- Create the schema at startup if it does not exist.
- Set the database path through env var `DB_PATH` (default `/data/homeoffice.db`).

## HTTP interface

| Method | Path | Behavior |
|---|---|---|
| `GET` | `/` | Main page. Optional query `?month=YYYY-MM` selects the calendar month. The default is the month of Today. |
| `POST` | `/today` | Mark Today. Redirect to `/` (`303`). |
| `POST` | `/days/{date}` | Mark `{date}` (`YYYY-MM-DD`). Redirect back to the month of `{date}` (`303`). |
| `POST` | `/days/{date}/delete` | Unmark `{date}`. Redirect back to the month of `{date}` (`303`). |
| `GET` | `/export/{year}` | CSV download for that year. |

- An invalid date format gives `400`. A future date on a mark request gives `400`.
- An invalid `?month=` value gives `400`. A month after the month of Today redirects to `/` (`303`).
- Use Post/Redirect/Get for all writes, so a browser reload does not resubmit a form.

## Main page

The page has three parts, from top to bottom:

1. **Today panel**
   - If Today is not marked: one large button **"Mark today as home office day"**.
   - If Today is marked: the text **"Today is a home office day"** and a small **"Undo"** button. "Undo" unmarks Today.
   - Opening the page never marks Today by itself.

2. **Month calendar**
   - A grid of the selected month, with weeks starting on Monday.
   - Links to the previous and the next month. The "next" link is hidden when the next month starts after Today.
   - Each day is one form button:
     - Not marked: a click marks the day.
     - Marked: filled style. A click unmarks the day.
     - After Today: disabled, not clickable.
   - Today has a visible outline.

3. **Year counts table**
   - One row per year with at least one Home office day, newest year first.
   - The current year always shows, also with a count of 0. It is highlighted.
   - Each row has a **"CSV"** link to `/export/{year}`.

## Export

- `Content-Type: text/csv; charset=utf-8`
- `Content-Disposition: attachment; filename="homeoffice-{year}.csv"`
- Header row, then one row per Home office day, oldest first:

```csv
date,weekday
2026-01-05,Monday
2026-01-07,Wednesday
```

- A year without Home office days gives a file with only the header row.

## Container (Podman)

The owner runs Podman on macOS (verified: Podman 6.0.2, `libkrun` machine, rootful). `podman compose` uses Docker Compose v5 as its provider, so a standard `compose.yaml` works.

**Containerfile** (multi-stage):

- Build stage: `docker.io/library/golang` (current stable tag), `CGO_ENABLED=0 go build`.
- Run stage: `gcr.io/distroless/static-debian12:nonroot`. Copy only the binary.
- Use fully qualified image names (registry included). Podman does not default to Docker Hub for short names.
- The app listens on port `8080` in the container.

**compose.yaml**:

```yaml
services:
  homeoffice:
    build: .
    ports:
      - "127.0.0.1:40000:8080"
    environment:
      TZ: Europe/Berlin
    volumes:
      - ./data:/data
    restart: unless-stopped
```

- Use a **bind mount** (`./data`), not a named volume. `podman compose down -v` deletes named volumes but leaves a bind mount alone. The owner can also see and copy `./data/homeoffice.db` as a backup.
- Commit `data/.gitkeep`, so the directory exists before the first `up`. Podman then does not create it with the wrong owner. In `.gitignore`, add `data/*` and `!data/.gitkeep`.
- The `nonroot` user (uid 65532) can create files in `./data`. This is verified on the owner's Podman machine: uid 65532 created a database file and a journal file in a bind-mounted macOS folder, and the files show on the host with the owner's uid. No `chown`, `:U`, or `userns` option is necessary.
- Restart after reboot: `podman-restart.service` is enabled in the Podman machine. It starts `unless-stopped` containers again when the machine boots, unless the owner stopped them before. The Podman machine itself must start at macOS login (Podman Desktop setting, or `podman machine start`).

## Tests

Unit tests with `go test`:

- Date rules: reject future dates, reject invalid formats, accept Today, accept weekends.
- Marking is idempotent, and unmarking works.
- Year counts: correct count per year, counts at the year boundary (31 Dec / 1 Jan), the current year shows with 0.
- CSV output: header, order, weekday names, empty year.

Run store tests against a temporary SQLite file. Inject the clock (a "now" function) so that tests do not depend on the real date.

There is no end-to-end test. A manual smoke check after `podman compose up` is enough:

1. Run `podman compose up -d --build`. Check that `data/homeoffice.db` exists.
1. Open `http://localhost:40000`.
2. Mark Today.
3. Mark one past day in the calendar.
4. Download the CSV.
5. Run `podman compose down && podman compose up -d --build`.
6. Check that the data is still there.

## README

Short. It covers start (`podman compose up -d --build`), the URL, the location of the data file, and how to back it up. It also states that the app comes back after a reboot only if the Podman machine starts at login.

## Acceptance criteria

- [ ] `podman compose up -d --build` serves the app on `http://localhost:40000`. Other machines on the network cannot reach it.
- [ ] One click marks Today. "Undo" unmarks it.
- [ ] The owner can mark and unmark any past date through the month calendar.
- [ ] Future dates cannot be marked.
- [ ] The Year counts table shows every year with data, plus the current year.
- [ ] The CSV export per year matches the format above.
- [ ] The data survives `podman compose down`, `down -v`, image rebuilds, and Podman machine restarts.
- [ ] A click at 00:30 local time records the local date, not the UTC date.
- [ ] `go test ./...` passes.

## Out of scope

- Other day types (office, vacation, sick).
- Tax amounts, the *Homeoffice-Pauschale* cap (210 days / €6), or other tax rules.
- Login, multiple users, access from other devices.
- Automatic backups.
- An export of all years in one file.
- Weekend or public holiday warnings.

## Comments

### 2026-10-14: Deviations during implementation

- `compose.yaml` uses `build: { context: ., dockerfile: Containerfile }`, not `build: .`. Docker Compose v5 looks for a `Dockerfile` by default. With `build: .`, `podman compose up -d --build` fails with `lstat .../Dockerfile: no such file or directory`.
- The repo has a `.dockerignore` file. Docker Compose packs the build context on the client side and reads only `.dockerignore`, not `.containerignore`.
- A new route `POST /today/delete` unmarks Today and redirects to `/` (`303`). The "Undo" button in the Today panel posts to it. The server resolves Today when the owner clicks. With `/days/{date}/delete`, the page would send the date from the time it rendered. A click after midnight would then unmark the wrong day.
