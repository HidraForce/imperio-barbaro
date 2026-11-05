importScripts('/pwa-state-worker.js');
const CACHE='imperio-members-v3';
const SHELL=['/app','/member.js','/member.css','/pwa-state.js','/member.webmanifest','/app-icon-192.png','/app-icon-512.png','/app-icon-180.png','/notification-badge.png'];
self.addEventListener('install',event=>{event.waitUntil(caches.open(CACHE).then(cache=>cache.addAll(SHELL)).then(()=>self.skipWaiting()))});
self.addEventListener('activate',event=>{event.waitUntil(caches.keys().then(keys=>Promise.all(keys.filter(key=>key.startsWith('imperio-members-')&&key!==CACHE).map(key=>caches.delete(key)))).then(()=>self.clients.claim()))});
async function memberShell(request,path){
 const cache=await caches.open(CACHE);
 try{
  const response=await fetch(request,{cache:'no-store'});
  if(response.ok){try{await cache.put(path,response.clone())}catch{}return response}
  return await cache.match(path)||response;
 }catch(error){const saved=await cache.match(path);if(saved)return saved;throw error}
}
self.addEventListener('fetch',event=>{
 if(event.request.method!=='GET')return;
 const url=new URL(event.request.url);
 if(url.origin!==self.location.origin)return;
 if(url.pathname==='/app'&&event.request.mode==='navigate'){event.respondWith(memberShell(event.request,'/app'));return}
 if(SHELL.includes(url.pathname)&&url.pathname!=='/app'){event.respondWith(memberShell(event.request,url.pathname))}
});
self.addEventListener('push',event=>{
 let data={};try{data=event.data?.json()||{}}catch{}
 const options={body:data.body||'Abra o app para conferir o aviso do seu clã.',icon:'/app-icon-192.png',badge:'/notification-badge.png',tag:data.id||'imperio-aviso',renotify:true,requireInteraction:true,silent:false,vibrate:[220,100,220],data:{url:safePushURL(data.url)}};
 event.waitUntil(self.registration.showNotification(data.title||'📣 Império Bárbaro',options));
});
self.addEventListener('notificationclick',event=>{
 event.notification.close();const url=safePushURL(event.notification.data?.url);
 event.waitUntil(self.clients.matchAll({type:'window',includeUncontrolled:true}).then(async clients=>{for(const client of clients){if(new URL(client.url).origin===self.location.origin&&new URL(client.url).pathname==='/app'){await client.navigate(url);return client.focus()}}return self.clients.openWindow(url)}));
});
