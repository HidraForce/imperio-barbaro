importScripts('/pwa-state-worker.js');
const CACHE='imperio-members-v1';
const SHELL=['/app','/member.js','/member.css','/pwa-state.js','/member.webmanifest','/app-icon-192.png','/app-icon-512.png','/app-icon-180.png'];
self.addEventListener('install',event=>{event.waitUntil(caches.open(CACHE).then(cache=>cache.addAll(SHELL)).then(()=>self.skipWaiting()))});
self.addEventListener('activate',event=>{event.waitUntil(caches.keys().then(keys=>Promise.all(keys.filter(key=>key.startsWith('imperio-members-')&&key!==CACHE).map(key=>caches.delete(key)))).then(()=>self.clients.claim()))});
self.addEventListener('fetch',event=>{
 if(event.request.method!=='GET')return;
 const url=new URL(event.request.url);
 if(url.origin!==self.location.origin)return;
 if(url.pathname==='/app'&&event.request.mode==='navigate'){event.respondWith(fetch(event.request).catch(()=>caches.match('/app')));return}
 if(SHELL.includes(url.pathname)&&url.pathname!=='/app'){event.respondWith(fetch(event.request).catch(()=>caches.match(url.pathname)))}
});
self.addEventListener('push',event=>{
 let data={};try{data=event.data?.json()||{}}catch{}
 const options={body:data.body||'Abra o app para conferir o aviso do seu clã.',icon:'/app-icon-192.png',badge:'/app-icon-192.png',tag:data.id||'imperio-aviso',renotify:true,requireInteraction:true,silent:false,vibrate:[220,100,220],data:{url:safePushURL(data.url)}};
 event.waitUntil(self.registration.showNotification(data.title||'📣 Império Bárbaro',options));
});
self.addEventListener('notificationclick',event=>{
 event.notification.close();const url=safePushURL(event.notification.data?.url);
 event.waitUntil(self.clients.matchAll({type:'window',includeUncontrolled:true}).then(async clients=>{for(const client of clients){if(new URL(client.url).origin===self.location.origin&&new URL(client.url).pathname==='/app'){await client.navigate(url);return client.focus()}}return self.clients.openWindow(url)}));
});
