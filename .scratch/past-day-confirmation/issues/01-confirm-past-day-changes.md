# 01: Confirm changes to past days

Status: ready-for-agent

## Problem

In the month calendar, one tap on a **Past day** marks or unmarks it at once. The owner often taps the wrong date by accident. The owner wants a confirmation step before a **Past day** changes.

Use the terms from [CONTEXT.md](../../../CONTEXT.md): **Today**, **Past day**, **Selection**.

## Decisions

1. **Today** stays one-tap. This applies to the Today panel ("Mark today…" and "Undo") and to the **Today** cell in the calendar.
2. A tap on a **Past day** writes nothing. It adds the date to the **Selection** or removes it from the **Selection**.
3. The **Selection** lives only in the URL. The app stays without JavaScript. Plain HTML forms do all writes.
4. A **Selection** can contain marked and unmarked dates. Confirm unmarks each selected marked date and marks each selected unmarked date.
5. The **Selection** stays when the owner goes to the previous or the next month.
6. A tap on **Today** (panel or cell) changes Today and keeps the **Selection** and the shown month.
7. The server enforces the confirmation. The one-tap route only writes **Today**. For a **Past day**, it writes nothing and puts the date into the **Selection**.
8. No ADR. The spec already explains the "no JavaScript" rule.

## Behavior

### Main page: `GET /`

- New repeatable query parameter `select=YYYY-MM-DD`. Example: `/?month=2026-10&select=2026-09-30&select=2026-10-01`.
- The server ignores `select` values that are invalid, after **Today**, or equal to **Today**. It also removes duplicates. These values do not give `400`.
- Each **Past day** cell is a link, not a form. The link goes to the same page with that date added to the `select` list or removed from it.
- A selected cell shows the state that it gets after Confirm. Example: a dashed outline in the marked color for a selected unmarked date. The selected style must look different from the marked style and from the unmarked style.
- The "Previous" and "Next" links keep all `select` values.
- The **Today** cell and the cells after **Today** do not change.

### Confirmation bar

- The bar shows only when the **Selection** is not empty.
- The bar is fixed at the bottom of the screen. It must not hide the last rows of the page. Add bottom padding to the page.
- The bar shows a summary and the dates, oldest first, with the year. Example: "Mark 3 days, unmark 1 day: Mon 6 Oct 2026, Tue 7 Oct 2026, Wed 8 Oct 2026, ~~Wed 1 Oct 2026~~". Show the dates to unmark struck through.
- Use singular and plural correctly ("1 day", "2 days"). Leave out a part with zero dates ("Mark 2 days", not "Mark 2 days, unmark 0 days").
- **Confirm** is a form button. It posts to `POST /selection`.
- **Cancel** is a link to `/?month=<shown month>`. It clears the whole **Selection**.

### New route: `POST /selection`

- Form fields: repeatable `mark` and `unmark` (`YYYY-MM-DD`), and `month` (`YYYY-MM`, the shown month).
- The page sends explicit `mark` and `unmark` lists, not "toggle these dates". An old page in a second tab then cannot reverse a change.
- Validation: each date must be a valid **Past day**. An invalid date, **Today**, or a date after **Today** gives `400`. The server writes nothing in this case.
- A date in both lists gives `400`.
- An empty request (no `mark` and no `unmark`) redirects without a write.
- Write all changes in one SQLite transaction. Add one store method for this, for example `Store.Apply(mark, unmark []civil.Date) error`.
- Redirect to `/?month=<month>` (`303`), without `select`. The owner sees the month that was shown when the owner tapped Confirm.
- Mark stays idempotent. Unmark of an unmarked date does nothing.

### Changed routes: `POST /days/{date}` and `POST /days/{date}/delete`

- Only the **Today** cell in the calendar uses these routes now.
- If `{date}` is **Today**: mark or unmark it, as before.
- If `{date}` is a **Past day**: write nothing. Redirect (`303`) to the calendar with `{date}` added to the **Selection**. This handles a stale page after midnight: the cell still shows yesterday as Today.
- A date after **Today** still gives `400`. An invalid date still gives `400`.
- The forms send hidden fields `month` and `select` (the current **Selection**). The redirect goes to `/?month=<month>&select=…`, so the **Selection** and the shown month stay.

### Changed routes: `POST /today` and `POST /today/delete`

- The Today panel forms also send hidden fields `month` and `select`.
- Redirect to `/?month=<month>&select=…` (`303`). Without these fields, redirect to `/` as before.
- Validate `month` like the `?month=` query. Filter `select` like on the main page.

## Tests

Add handler tests in [web_test.go](../../../internal/web/web_test.go). Use the injected clock. Change the existing tests that expect a one-tap write for a **Past day**: `TestOwnerMarksPastDaysIncludingWeekends`, `TestOwnerUnmarksAWrongDay`, `TestCalendarDaysToggleTheirMark`.

- A **Past day** cell links to the page with the date added to `select`. A selected cell links to the page with the date removed.
- The bar shows only with a non-empty **Selection**. It shows the correct summary for mark only, unmark only, and mixed.
- `select` values that are invalid, after **Today**, or equal to **Today** are ignored.
- The month navigation links keep the **Selection**.
- `POST /selection` with mixed `mark` and `unmark` writes all changes and redirects to the shown month.
- `POST /selection` with a date from another month works.
- `POST /selection` with **Today**, a future date, an invalid date, or a date in both lists gives `400` and writes nothing.
- `POST /days/{date}` for a **Past day** writes nothing and redirects with the date in `select` (stale **Today** after midnight).
- `POST /days/{date}` and `POST /today` for **Today** keep the existing **Selection** and month in the redirect.
- Store: `Apply` writes marks and unmarks together.

## Acceptance criteria

- [ ] A tap on a **Past day** changes no data. It only changes the **Selection**.
- [ ] The owner can select **Past days** across months and confirm them with one tap.
- [ ] One **Selection** can mark and unmark dates at the same time.
- [ ] Cancel clears the **Selection** and changes no data.
- [ ] **Today** stays one-tap in the Today panel and in the calendar. It keeps the **Selection**.
- [ ] A stale **Today** cell after midnight does not write the old date. It puts the date into the **Selection**.
- [ ] The page uses no JavaScript.
- [ ] The confirmation bar is visible on a phone screen without scrolling.
- [ ] `go test ./...` passes.

## Out of scope

- A confirmation for **Today**.
- An undo after Confirm.
- A JavaScript dialog or `confirm()`.
