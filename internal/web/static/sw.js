self.addEventListener("install", () => self.skipWaiting());

self.addEventListener("activate", (event) => event.waitUntil(self.clients.claim()));

self.addEventListener("push", (event) => {
  const { title, body } = event.data.json();
  event.waitUntil(
    self.registration.showNotification(title, { body, tag: "reminder", icon: "/apple-touch-icon.png" }),
  );
});

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  event.waitUntil(openTracker());
});

async function openTracker() {
  const windows = await self.clients.matchAll({ type: "window", includeUncontrolled: true });
  const tracker = windows.find((client) => new URL(client.url).origin === self.location.origin);
  if (!tracker) {
    return self.clients.openWindow("/");
  }
  await tracker.focus();
  try {
    await tracker.navigate("/");
  } catch {
    // An uncontrolled window cannot be navigated; it stays focused as it is.
  }
}
