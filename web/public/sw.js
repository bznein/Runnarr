const CACHE_NAME = "runnarr-shell-v3";
const SHELL_URLS = [
  "/",
  "/index.html",
  "/manifest.webmanifest",
  "/icon.svg",
  "/icon-192.png",
  "/icon-512.png",
  "/apple-touch-icon.png"
];

self.addEventListener("install", (event) => {
  event.waitUntil(
    caches.open(CACHE_NAME).then((cache) => cache.addAll(SHELL_URLS)).then(() => self.skipWaiting())
  );
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches.keys().then((keys) => Promise.all(
      keys.filter((key) => key !== CACHE_NAME).map((key) => caches.delete(key))
    )).then(() => self.clients.claim())
  );
});

self.addEventListener("message", (event) => {
  if (event.data?.type === "SKIP_WAITING") {
    void self.skipWaiting();
  }
});

self.addEventListener("fetch", (event) => {
  const request = event.request;
  const url = new URL(request.url);

  if (
    request.method !== "GET" ||
    url.origin !== self.location.origin ||
    url.pathname.startsWith("/api/") ||
    url.pathname === "/sw.js"
  ) {
    return;
  }

  if (request.mode === "navigate") {
    event.respondWith(
      fetch(request).then((response) => {
        const copy = response.clone();
        void caches.open(CACHE_NAME).then((cache) => cache.put("/index.html", copy));
        return response;
      }).catch(() => caches.match("/index.html"))
    );
    return;
  }

  if (["script", "style", "font", "image"].includes(request.destination)) {
    event.respondWith(
      caches.match(request).then((cached) => {
        const network = fetch(request).then((response) => {
          if (response.ok) {
            const copy = response.clone();
            void caches.open(CACHE_NAME).then((cache) => cache.put(request, copy));
          }
          return response;
        }).catch(() => cached);
        return cached || network;
      })
    );
  }
});

self.addEventListener("push", (event) => {
  let data = {};
  try {
    data = event.data?.json() ?? {};
  } catch {
    data = { title: "Runnarr has an update", actionPath: "/notifications" };
  }
  const title = data.title || "Runnarr has an update";
  const actionPath = typeof data.actionPath === "string" && data.actionPath.startsWith("/") && !data.actionPath.startsWith("//")
    ? data.actionPath
    : "/notifications";
  const badgePromise = typeof self.navigator?.setAppBadge === "function" && Number(data.unreadCount) > 0
    ? self.navigator.setAppBadge(Number(data.unreadCount))
    : Promise.resolve();
  event.waitUntil(Promise.all([
    self.registration.showNotification(title, {
      body: data.body || "Open Runnarr for details.",
      icon: "/icon-192.png",
      badge: "/icon-192.png",
      tag: data.tag || "runnarr-notification",
      renotify: true,
      data: { actionPath, notificationId: data.notificationId || "" }
    }),
    badgePromise
  ]));
});

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const actionPath = event.notification.data?.actionPath || "/notifications";
  const notificationId = event.notification.data?.notificationId;
  const navigate = self.clients.matchAll({ type: "window", includeUncontrolled: true }).then(async (clients) => {
    const target = new URL(actionPath, self.location.origin);
    if (notificationId) target.searchParams.set("runnarrNotification", notificationId);
    const targetURL = target.href;
    for (const client of clients) {
      if ("navigate" in client) await client.navigate(targetURL);
      return client.focus();
    }
    return self.clients.openWindow(targetURL);
  });
  event.waitUntil(navigate);
});
