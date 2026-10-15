(async () => {
  const line = document.querySelector(".reminder");
  if (!line || !("serviceWorker" in navigator) || !("PushManager" in window) || !("Notification" in window)) {
    return;
  }

  const enableButton = line.querySelector(".reminder-enable");
  const onText = line.querySelector(".reminder-on");
  const offButton = line.querySelector(".reminder-off");
  const blockedText = line.querySelector(".reminder-blocked");
  await navigator.serviceWorker.register("/sw.js");
  const registration = await navigator.serviceWorker.ready;

  async function post(path, body) {
    const response = await fetch(path, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    if (!response.ok) {
      throw new Error(`${path}: ${response.status}`);
    }
  }

  function show(state) {
    enableButton.hidden = state !== "enable";
    onText.hidden = state !== "on";
    blockedText.hidden = state !== "blocked";
    line.hidden = false;
  }

  async function render() {
    if (Notification.permission === "denied") {
      show("blocked");
      return;
    }
    const subscription = await registration.pushManager.getSubscription();
    show(subscription ? "on" : "enable");
    if (subscription) {
      // Re-send the subscription, so the server knows it also after a lost database.
      await post("/reminder/subscriptions", subscription.toJSON()).catch((error) =>
        console.error("reminder setup:", error),
      );
    }
  }

  async function run(action) {
    try {
      await action();
    } catch (error) {
      console.error("reminder setup:", error);
    }
    await render();
  }

  enableButton.addEventListener("click", () =>
    run(async () => {
      if ((await Notification.requestPermission()) !== "granted") {
        return;
      }
      const subscription = await registration.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: line.dataset.vapidPublicKey,
      });
      await post("/reminder/subscriptions", subscription.toJSON());
    }),
  );

  offButton.addEventListener("click", () =>
    run(async () => {
      const subscription = await registration.pushManager.getSubscription();
      if (!subscription) {
        return;
      }
      await post("/reminder/subscriptions/delete", { endpoint: subscription.endpoint });
      await subscription.unsubscribe();
    }),
  );

  await render();
})();
