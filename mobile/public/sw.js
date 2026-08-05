// sw.js — Service Worker: Web Push уведомления + badge sync

self.addEventListener("push", function (event) {
  if (!event.data) return;

  /** @type {any} */
  let data;
  try {
    data = event.data.json();
  } catch {
    data = { title: "my-chat", body: event.data.text() };
  }

  // badge_sync: тихое обновление бейджа без уведомления
  if (data.event_type === "badge_sync" || data.type === "badge_sync") {
    const badge = typeof data.badge === "number" ? data.badge : 0;
    event.waitUntil(
      Promise.resolve().then(() => {
        if ("setAppBadge" in self.navigator) {
          return self.navigator.setAppBadge(badge);
        }
      })
    );
    return;
  }

  // Обычное push-уведомление: message_new и т.п.
  const title = data.title || "my-chat";
  const body = data.body || data.preview || "";
  const dialogId = data.dialog_id || "";
  const badge = typeof data.badge === "number" ? data.badge : undefined;

  const options = {
    body,
    icon: "/icons/icon-192.png",
    badge: "/icons/icon-192.png",
    data: { dialog_id: dialogId },
    tag: dialogId ? `dialog-${dialogId}` : "mychat",
    renotify: Boolean(dialogId),
  };

  event.waitUntil(
    Promise.all([
      self.registration.showNotification(title, options),
      badge !== undefined && "setAppBadge" in self.navigator
        ? self.navigator.setAppBadge(badge)
        : Promise.resolve(),
    ])
  );
});

self.addEventListener("notificationclick", function (event) {
  event.notification.close();

  const dialogId =
    event.notification.data && event.notification.data.dialog_id;
  const targetUrl = dialogId
    ? `/?dialog=${encodeURIComponent(dialogId)}`
    : "/";

  event.waitUntil(
    self.clients
      .matchAll({ type: "window", includeUncontrolled: true })
      .then(function (clientList) {
        // Если приложение уже открыто — фокусируемся и передаём сообщение
        for (const client of clientList) {
          if ("focus" in client) {
            void client.focus();
            if (dialogId) {
              client.postMessage({ type: "open_dialog", dialog_id: dialogId });
            }
            return client;
          }
        }
        // Иначе открываем новую вкладку
        if (self.clients.openWindow) {
          return self.clients.openWindow(targetUrl);
        }
      })
  );
});
