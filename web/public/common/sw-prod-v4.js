self.addEventListener('install', () => {
  self.skipWaiting();
});

self.addEventListener('activate', event => {
  event.waitUntil((async () => {
    for (const name of await caches.keys()) {
      await caches.delete(name);
    }
    await self.registration.unregister();
    for (const client of await self.clients.matchAll({type: 'window'})) {
      client.navigate(client.url);
    }
  })());
});
