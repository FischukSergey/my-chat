// sw.js — Service Worker: Web Push уведомления + badge sync
//
// iOS/WebKit: подписка всегда userVisibleOnly=true → каждый push ОБЯЗАН
// вызвать showNotification(), иначе бейдж/подписка ведут себя непредсказуемо.
// Тихий badge_sync без уведомления на iOS не гарантирован.

self.addEventListener("push", function (event) {
  if (!event.data) return;

  /** @type {any} */
  let data;
  try {
    data = event.data.json();
  } catch {
    data = { title: "my-chat", body: event.data.text() };
  }

  // badge_sync: обновить бейдж (+ silent notification ради userVisibleOnly)
  if (data.event_type === "badge_sync" || data.type === "badge_sync") {
    const badge = typeof data.badge === "number" ? data.badge : 0;
    event.waitUntil(
      Promise.all([
        syncWorkerBadge(badge),
        self.registration
          .showNotification("my-chat", {
            body: badge > 0 ? `Непрочитанных: ${badge}` : "Все прочитано",
            silent: true,
            tag: "badge-sync",
            renotify: false,
            icon: "/icons/icon-192.png",
          })
          .then(function () {
            // Сразу закрываем, чтобы не копить шум в шторке
            return self.registration.getNotifications({ tag: "badge-sync" }).then(function (list) {
              list.forEach(function (n) {
                n.close();
              });
            });
          }),
      ])
    );
    return;
  }

  // Обычное push-уведомление: message_new и т.п.
  const title = data.title || "my-chat";
  const body = data.body || data.preview || "";
  const dialogId = data.dialog_id || "";
  const badge = typeof data.badge === "number" ? data.badge : undefined;

  const options = {
    body: body,
    icon: "/icons/icon-192.png",
    badge: "/icons/icon-192.png",
    data: { dialog_id: dialogId },
    tag: dialogId ? "dialog-" + dialogId : "mychat",
    renotify: Boolean(dialogId),
  };

  event.waitUntil(
    Promise.all([
      self.registration.showNotification(title, options),
      badge !== undefined ? syncWorkerBadge(badge) : Promise.resolve(),
    ])
  );
});

/**
 * @param {number} badge
 * @returns {Promise<void>}
 */
function syncWorkerBadge(badge) {
  if (badge > 0) {
    if ("setAppBadge" in self.navigator) {
      return self.navigator.setAppBadge(badge);
    }
    return Promise.resolve();
  }
  if ("clearAppBadge" in self.navigator) {
    return self.navigator.clearAppBadge();
  }
  if ("setAppBadge" in self.navigator) {
    return self.navigator.setAppBadge(0);
  }
  return Promise.resolve();
}

self.addEventListener("notificationclick", function (event) {
  event.notification.close();

  const dialogId =
    event.notification.data && event.notification.data.dialog_id;
  const targetUrl = dialogId
    ? "/?dialog=" + encodeURIComponent(dialogId)
    : "/";

  event.waitUntil(
    self.clients
      .matchAll({ type: "window", includeUncontrolled: true })
      .then(function (clientList) {
        for (const client of clientList) {
          if ("focus" in client) {
            void client.focus();
            if (dialogId) {
              client.postMessage({ type: "open_dialog", dialog_id: dialogId });
            }
            return client;
          }
        }
        if (self.clients.openWindow) {
          return self.clients.openWindow(targetUrl);
        }
      })
  );
});
