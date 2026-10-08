const CACHE = 'clanboard-v2';
self.addEventListener('install', () => self.skipWaiting());
self.addEventListener('activate', event => event.waitUntil(
  caches.keys().then(keys => Promise.all(keys.filter(key => key.startsWith('clanboard-')).map(key => caches.delete(key))))
    .then(() => self.clients.claim())
));
// Páginas autenticadas e respostas da API consultam o servidor; o cache da API fica no Go.
self.addEventListener('fetch', event => {
  const url = new URL(event.request.url);
  if (event.request.method !== 'GET' || url.origin !== self.location.origin) return;
  if (url.pathname.startsWith('/api/') || event.request.mode === 'navigate') {
    event.respondWith(fetch(event.request, { cache: 'no-store' }));
  }
});
