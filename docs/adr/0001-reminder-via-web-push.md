# Reminder through Web Push, not a native macOS notification

The owner wants a **Reminder** on weekday mornings, also when no tab of the app is open. We use Web Push: the Go server signs a push message with its own VAPID keys and sends it through the push service of the browser. A service worker in the browser shows the **Reminder**. We did not choose a native macOS notification from a `launchd` job, although it needs no JavaScript and no browser support. Web Push keeps the feature inside the container and the repo, it needs no setup on the host, and it works like the notifications of other web apps.

## Consequences

- The app now has JavaScript (a service worker and one small script). It is only for **Reminder** setup. All writes stay plain HTML forms, and the page works without JavaScript.
- The container needs outbound internet access to reach the push service.
- The browser app must run (a window is not necessary) to show a **Reminder**.
- The owner uses Arc. Arc is based on Chromium but gets only maintenance updates. We did not test Web Push in Arc before the decision. If Arc does not show a **Reminder**, go back to the native macOS notification.
- If the VAPID keys change, all subscriptions stop working. The server stores the keys in the SQLite file, so the normal backup keeps them.
