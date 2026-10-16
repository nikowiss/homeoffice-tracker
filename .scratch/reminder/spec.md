# Spec: Weekday Reminder

Status: ready-for-agent

## Goal

On every weekday morning, the browser shows a **Reminder** that asks the owner to mark **Today**. The **Reminder** also shows when no tab of the app is open. A click on the **Reminder** opens the app. The owner then marks **Today** as usual.

Use the terms from [CONTEXT.md](../../CONTEXT.md): **Today**, **Home office day**, **Reminder**. Read [ADR 0001](../../docs/adr/0001-reminder-via-web-push.md) for the reason for Web Push.

## Decisions

1. Delivery is Web Push: a service worker, the Push API, and VAPID. The Go server sends the push. No tab must be open, but the browser app (Arc) must run.
2. A **Reminder** changes nothing. A click opens the app. The notification has no action buttons.
3. The app sends a **Reminder** only from Monday to Friday, and only if **Today** is not marked. There is no public holiday or vacation logic.
4. The app sends at most one **Reminder** per day. A **Reminder** expires at midnight `Europe/Berlin`.
5. The env var `REMINDER_TIME` (`HH:MM`, `Europe/Berlin`) sets the time. The default is `09:00`.
6. The owner turns **Reminders** on and off with a button on the main page. There is no server-side "off" value.
7. The server sends to every stored subscription. It deletes a subscription when the push service answers `404` or `410`.
8. The server generates the VAPID keys on the first start and stores them in SQLite.
9. JavaScript is allowed for **Reminder** setup only. All writes of **Home office days** stay plain HTML forms. The page works fully without JavaScript.
10. Push library: `github.com/SherClockHolmes/webpush-go`. It is pure Go and works with `CGO_ENABLED=0`.

## Configuration

- `REMINDER_TIME`: `HH:MM` in 24-hour format, for example `09:00` or `08:30`. Empty or not set means `09:00`.
- An invalid value stops the server at startup with a clear error, for example `invalid REMINDER_TIME "9h": want HH:MM`.
- Add `REMINDER_TIME: "09:00"` to `environment` in `compose.yaml`.

## Data

Add to the schema at startup (`CREATE TABLE IF NOT EXISTS`):

```sql
CREATE TABLE IF NOT EXISTS push_subscriptions (
  endpoint TEXT PRIMARY KEY,
  p256dh   TEXT NOT NULL,
  auth     TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
```

- `settings` holds `vapid_public_key`, `vapid_private_key`, and `last_reminder_date` (ISO 8601 date).
- Generate the VAPID keys only if `vapid_public_key` does not exist. A restart, a rebuild, or a new image never changes the keys.
- Saving a subscription with an existing `endpoint` replaces its keys (upsert).

## Schedule

A goroutine checks once per minute. Do not use one long timer to the next reminder time. The container runs in the Podman VM, which pauses while the Mac sleeps, and a long timer can then fire at the wrong time.

A **Reminder** is due when all of these are true:

1. **Today** is Monday to Friday.
2. The current time in `Europe/Berlin` is at or after `REMINDER_TIME`.
3. **Today** is not a **Home office day**.
4. `last_reminder_date` is not **Today**.

This one rule also covers the late cases. If the server starts, or the Mac wakes, after the reminder time, the next check sends the **Reminder**.

When a **Reminder** is due:

- Send one push to each stored subscription.
- Set the push `TTL` to the seconds until the next midnight in `Europe/Berlin`. The push service then drops the **Reminder** at the end of **Today**.
- Set `last_reminder_date` to **Today** if at least one push service accepted the push (`201`).
- If no push was accepted (no subscriptions, or a network error), do not set `last_reminder_date`. The next check tries again. Note: if the owner turns **Reminders** on in the afternoon of an unmarked weekday, a **Reminder** comes within one minute. This is correct.
- Delete a subscription on `404` or `410`. Log other errors, and do not delete the subscription.

Write the "is due" rule and the TTL calculation as pure functions that take the time as input. Use the injected `now` function, as `web.NewHandler` does.

## Push message

- The payload is the JSON `{"title":"Home office today?","body":"Open the tracker to mark today."}`.
- The service worker shows the notification with `tag: "reminder"`. A new **Reminder** then replaces an old one.
- VAPID subscriber: `https://github.com/nikowiss/homeoffice-tracker`.

## HTTP interface

| Method | Path | Behavior |
|---|---|---|
| `GET` | `/sw.js` | The service worker. `Content-Type: text/javascript`. Served from the root, so its scope is `/`. |
| `GET` | `/reminder.js` | The script for the button on the main page. |
| `POST` | `/reminder/subscriptions` | JSON body from `PushSubscription.toJSON()`. Stores the subscription. Answers `204`. Invalid JSON, or a missing `endpoint`, `p256dh`, or `auth`, gives `400`. |
| `POST` | `/reminder/subscriptions/delete` | JSON body `{"endpoint": "..."}`. Deletes the subscription. Answers `204`, also if the subscription does not exist. |
| `POST` | `/reminder/test` | Sends a **Reminder** now to all subscriptions. It ignores the weekday, **Today**, and `last_reminder_date` rules, and does not change `last_reminder_date`. Answers `204`. This is for the manual smoke check (`curl -X POST http://localhost:40000/reminder/test`). |

Put the VAPID public key into the main page as a `data-` attribute. A separate endpoint is not necessary.

## Main page

Add one small line below the year counts table. Render it with `hidden`. `reminder.js` shows it, so a page without JavaScript shows nothing.

`reminder.js` registers `/sw.js` and shows one of these states:

- No subscription in this browser: a button **"Enable reminders"**. A click asks for permission, subscribes with the VAPID public key (`userVisibleOnly: true`), and posts the subscription to `/reminder/subscriptions`.
- A subscription exists: the text **"Reminders on"** and a small button **"Turn off"**. A click unsubscribes in the browser and posts the endpoint to `/reminder/subscriptions/delete`.
- Permission is `denied`: the text **"Notifications are blocked for this site in the browser settings."**
- No service worker or Push API support: keep the line hidden.

## Service worker

- `push`: show the notification with the title and body from the payload.
- `notificationclick`: close the notification. If a window of the app is open, focus it and load `/` in it. If not, open `/` in a new window.

## Tests

Unit tests with `go test`:

- "Is due" rule: before the time, at the time, after the time, Saturday, Sunday, **Today** marked, `last_reminder_date` is **Today**, `last_reminder_date` is yesterday.
- TTL: seconds until midnight `Europe/Berlin`, also on the days when daylight saving time starts and ends.
- `REMINDER_TIME` parsing: default, valid values, invalid values (`9:00`, `24:00`, `09:60`, `abc`).
- Store: subscription upsert and delete, VAPID keys stay the same after the store is opened again, `last_reminder_date` read and write.
- Handlers: `POST /reminder/subscriptions` with valid and invalid JSON.

Do not send real pushes in tests. Put the sender behind a small interface, and use a fake in tests to check `201`, `404`, `410`, and errors.

## README

Add a short section **Reminders**:

- Click "Enable reminders" on the main page and allow notifications.
- In macOS System Settings → Notifications → Arc, allow notifications.
- Arc must run at the reminder time. A window is not necessary. Focus modes can hide the **Reminder**.
- Change the time with `REMINDER_TIME` in `compose.yaml`.
- Test it with `curl -X POST http://localhost:40000/reminder/test`.

## Acceptance criteria

- [ ] `go test ./...` passes.
- [ ] Without JavaScript, the main page looks and works as before.
- [ ] "Enable reminders" in Arc asks for permission and stores one subscription.
- [ ] **Manual smoke check in Arc:** turn reminders on, close all Arc windows (do not quit Arc), and run `curl -X POST http://localhost:40000/reminder/test`. The **Reminder** shows. A click on it opens `http://localhost:40000`. **If this check fails, stop the work and report it. Do not try workarounds in Arc** (see ADR 0001).
- [ ] On a weekday with **Today** not marked, a **Reminder** comes at `REMINDER_TIME`, and only one.
- [ ] No **Reminder** comes on Saturday or Sunday, or when **Today** is marked.
- [ ] After a container restart on the same day, no second **Reminder** comes.
- [ ] "Turn off" deletes the subscription. After that, `/reminder/test` shows nothing in this browser.
- [ ] The VAPID keys stay the same after `podman compose down && podman compose up -d`. The subscription still works.

## Out of scope

- Action buttons in the notification (for example "Yes, home office").
- Public holidays, vacation, or other day types.
- A settings UI for the reminder time.
- Withdrawing a **Reminder** that is already shown when the owner marks **Today**.
- Notifications on other devices. The app stays bound to `127.0.0.1`.

## Comments

### 2026-10-15: Smoke check result

- Arc failed. Arc stored a subscription, and FCM accepted a probe push with `201`. But Arc never showed the **Reminder**. Arc is not in macOS System Settings → Notifications, so Arc does not pass web notifications on to macOS.
- Zen (based on Firefox, Mozilla push service) passed the smoke check. The owner moves to Zen. The code has no Arc-specific parts, so no code change was necessary.
- The main page now shows an error text below the button when "Enable reminders" or "Turn off" fails. Before, the error went only to the browser console, and a click seemed to do nothing.
