import test from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import vm from 'node:vm'
import { memberSetup, memberClan, memberAppURL } from '../public/pwa-state.js'

test('Untrusted clan values cannot change member navigation or inject query parameters',()=>{
 const origin='https://imperiobarbaro.com.br'
 for(const clan of ['1','2']){
  assert.equal(memberClan(clan),clan)
  const url=new URL(memberAppURL(clan),origin)
  assert.equal(url.origin,origin)
  assert.equal(url.pathname,'/app')
  assert.equal(url.searchParams.get('clan'),clan)
  assert.deepEqual([...url.searchParams.keys()],['clan'])
 }
 for(const value of [null,undefined,1,2,'','3',' 1','1&redirect=https://evil.test','//evil.test','javascript:alert(1)','<img src=x onerror=alert(1)>']){
  assert.equal(memberClan(value),'')
  assert.equal(memberAppURL(value),'/app')
 }
})

test('Installation gate requires notification permission and a saved push subscription',()=>{
 const base={ios:false,installed:false,supported:true,permission:'default',subscribed:false}
 assert.equal(memberSetup(base),'enable')
 assert.equal(memberSetup({...base,permission:'granted'}),'enable')
 assert.equal(memberSetup({...base,permission:'granted',subscribed:true}),'install')
 assert.equal(memberSetup({...base,permission:'granted',subscribed:true,installed:true}),'ready')
 assert.equal(memberSetup({...base,permission:'denied',subscribed:true,installed:true}),'blocked')
 assert.equal(memberSetup({...base,supported:false}),'unsupported')
 // iOS cannot request web push until installed: never leave users in a dead end.
 assert.equal(memberSetup({...base,ios:true,supported:false}),'install-first')
 assert.equal(memberSetup({...base,ios:true,installed:true}),'enable')
})

test('Worker displays closed-app pushes and only opens the member app on the same origin',async()=>{
 const listeners={},shown=[],opened=[],cacheWrites=[]
 const context=vm.createContext({URL,self:{location:{origin:'https://imperiobarbaro.com.br'},registration:{showNotification:(title,options)=>{shown.push({title,options});return Promise.resolve()}},clients:{matchAll:async()=>[],openWindow:url=>{opened.push(url);return Promise.resolve()}},addEventListener:(name,fn)=>listeners[name]=fn},caches:{open:async()=>({addAll:async list=>cacheWrites.push(...list)})}})
 context.importScripts=()=>vm.runInContext(context.helper,context)
 context.helper=await readFile(new URL('../public/pwa-state-worker.js',import.meta.url),'utf8')
 vm.runInContext(await readFile(new URL('../public/pwa-sw.js',import.meta.url),'utf8'),context)
 let pending
 listeners.push({data:{json:()=>({id:'stable-id',title:'Raids',body:'Use seus ataques!',url:'https://evil.test/'})},waitUntil:p=>pending=p});await pending
 assert.equal(shown[0].title,'Raids');assert.equal(shown[0].options.body,'Use seus ataques!');assert.equal(shown[0].options.tag,'stable-id');assert.equal(shown[0].options.silent,false)
 assert.equal(shown[0].options.badge,'/notification-badge.png')
 listeners.push({data:{json:()=>({id:'test-id',title:'Teste',body:'Teste neste celular',url:'/app'})},waitUntil:p=>pending=p});await pending
 for(const key of ['icon','badge','renotify','requireInteraction','silent'])assert.equal(shown[0].options[key],shown[1].options[key])
 assert.deepEqual([...shown[0].options.vibrate],[...shown[1].options.vibrate])
 listeners.notificationclick({notification:{close(){},data:{url:'https://evil.test/'}},waitUntil:p=>pending=p});await pending
 assert.equal(opened[0],'https://imperiobarbaro.com.br/app')
 listeners.notificationclick({notification:{close(){},data:{url:'/app?clan=2#abc'}},waitUntil:p=>pending=p});await pending
 assert.equal(opened[1],'https://imperiobarbaro.com.br/app?clan=2#abc')
 let intercepted=false
 listeners.fetch({request:{url:'https://imperiobarbaro.com.br/api/contacts',method:'GET'},respondWith(){intercepted=true}})
 assert.equal(intercepted,false,'private APIs must not be cached')
})

async function memberHarness({permission='granted',existing=true,saveFailure=false,clan='1',storageThrows=false,device,subscribeFailure=false}={}){
 device??={permission,existing,localClan:clan,savedClan:existing?clan:'',subscribeCalls:0,permissionRequests:0}
 let readFailure=false
 const elements=new Map(),listeners={},swListeners={}
 function element(id){if(!elements.has(id))elements.set(id,{value:'',hidden:false,disabled:false,textContent:'',classList:{add(){},toggle(){},contains(){return false}},addEventListener:(name,fn)=>listeners[id+':'+name]=fn,replaceChildren(){}});return elements.get(id)}
 let activate,registered
 const ready=new Promise(resolve=>activate=resolve),registeredPromise=new Promise(resolve=>registered=resolve)
 const requests=[],subscription={toJSON:()=>({endpoint:'device-only'})}
 const activeRegistration={pushManager:{getSubscription:async()=>{if(readFailure)throw Error('Não foi possível ler a inscrição agora.');return device.existing?subscription:null},subscribe:async()=>{device.subscribeCalls++;if(subscribeFailure)throw Error('Toque para reconectar.');device.existing=true;return subscription}}}
 const context=vm.createContext({memberSetup,memberClan,memberAppURL,URL,URLSearchParams,Uint8Array,atob,
  isSecureContext:true,PushManager:function(){},Notification:{get permission(){return device.permission},set permission(value){device.permission=value},requestPermission:async()=>{device.permissionRequests++;device.permission='granted';return 'granted'}},
  navigator:{userAgent:'Android',platform:'Linux',serviceWorker:{register:async()=>{registered();return {pushManager:{getSubscription(){throw Error('Inactive registration used')}}}},ready,addEventListener:(name,fn)=>swListeners[name]=fn}},
  document:{hidden:false,body:{dataset:{page:'install'},classList:{toggle(){}}},getElementById:element,createElement:()=>element(Symbol()),addEventListener:(name,fn)=>listeners['document:'+name]=fn},
  matchMedia:()=>({matches:true}),localStorage:{getItem:()=>{if(storageThrows)throw Error('Storage unavailable');return device.localClan},setItem:(key,value)=>{if(storageThrows)throw Error('Storage unavailable');device.localClan=value}},location:{search:'',pathname:'/instalar',hash:''},history:{replaceState(){}},
  setInterval(){return 1},fetch:async(path,options)=>{requests.push({path,options});if(path==='/api/push/subscription'&&saveFailure){if(saveFailure==='offline')throw Error('Sem conexão.');return {ok:false,json:async()=>({error:'O servidor está indisponível.'})}}if(path==='/api/push/subscription')device.savedClan=JSON.parse(options.body).clanId;return {ok:true,json:async()=>path==='/api/push/config'?{publicKey:'AA'}:path==='/api/push/subscription/restore'?{clanId:device.savedClan}:{accepted:true}}},addEventListener:(name,fn)=>listeners['window:'+name]=fn,
 })
 context.window=context
 const source=await readFile(new URL('../public/member.js',import.meta.url),'utf8')
 vm.runInContext(source.replace(/^import .*\n/,''),context)
 await registeredPromise
 return {elements,listeners,requests,context,device,setSaveFailure:value=>saveFailure=value,setReadFailure:value=>readFailure=value,setExisting:value=>device.existing=value,setSubscribeFailure:value=>subscribeFailure=value,activate:async()=>{activate(activeRegistration);await vm.runInContext('bootPromise',context)}}
}

test('Installed app waits for the active worker and recognizes existing permission without another tap or reload',async()=>{
 const app=await memberHarness()
 assert.equal(app.elements.get('enable-notifications').hidden,true)
 assert.match(app.elements.get('setup-status').textContent,/Verificando/)
 await app.activate()
 assert.equal(app.elements.get('enable-notifications').hidden,true)
 assert.equal(app.elements.get('setup-status').textContent,'Todos os avisos estão ativados.')
 assert.equal(app.requests.filter(x=>x.path==='/api/push/subscription').length,1)
 await app.listeners['test-notification:click']()
 const sent=app.requests.at(-1)
 assert.equal(sent.path,'/api/push/test')
 assert.deepEqual(JSON.parse(sent.options.body),{subscription:{endpoint:'device-only'}})
 // Returning from system settings updates the UI without a manual refresh.
 app.context.Notification.permission='denied'
 await app.listeners['window:focus']()
 assert.equal(app.elements.get('setup-status').textContent,'Seus avisos estão bloqueados.')
})

test('First activation immediately completes the installed app setup',async()=>{
 const app=await memberHarness({permission:'default',existing:false})
 await app.activate()
 assert.equal(app.elements.get('enable-notifications').hidden,false)
 await app.listeners['enable-notifications:click']()
 assert.equal(app.elements.get('enable-notifications').hidden,true)
 assert.equal(app.elements.get('setup-status').textContent,'Todos os avisos estão ativados.')
 assert.equal(app.elements.get('test-notification').hidden,false)
})

test('Returning from the dashboard with a failed sync preserves the subscription and never offers activation',async()=>{
 for(const failure of ['http','offline']){
  const app=await memberHarness()
  await app.activate()
  app.context.document.hidden=true
  await app.listeners['document:visibilitychange']()
  app.setSaveFailure(failure)
  app.context.document.hidden=false
  await app.listeners['document:visibilitychange']()
  assert.equal(app.context.Notification.permission,'granted')
  assert.equal(app.elements.get('enable-notifications').hidden,true)
  assert.equal(app.elements.get('setup-status').textContent,'Permissão ativa. Verificação pendente.')
  assert.equal(app.elements.get('retry-setup').hidden,false)
  assert.equal(vm.runInContext('subscribed',app.context),true,'a connection error must not clear an already confirmed enrollment')
  assert.ok(!app.requests.some(request=>request.options.method==='DELETE'))
  app.setSaveFailure(false)
  await app.listeners['retry-setup:click']()
  assert.equal(app.elements.get('setup-status').textContent,'Todos os avisos estão ativados.')
  assert.equal(app.elements.get('enable-notifications').hidden,true)
  assert.equal(app.elements.get('action-feedback').textContent,'')
 }
})

test('A temporary PushManager failure requests a recheck without presenting notifications as disabled',async()=>{
 const app=await memberHarness()
 await app.activate()
 app.setReadFailure(true)
 await app.listeners['window:focus']()
 assert.equal(app.elements.get('enable-notifications').hidden,true)
 assert.equal(app.elements.get('setup-status').textContent,'Permissão ativa. Verificação pendente.')
 app.setReadFailure(false)
 await app.listeners['window:focus']()
 assert.equal(app.elements.get('setup-status').textContent,'Todos os avisos estão ativados.')
 // With permission still granted, a missing subscription is repaired.
 app.setExisting(false)
 await app.listeners['window:focus']()
 assert.equal(app.elements.get('enable-notifications').hidden,true)
 assert.equal(app.device.subscribeCalls,1)
 assert.equal(app.device.permissionRequests,0)
})

test('An existing device with a failing initial enrollment sync offers retry, not another permission request',async()=>{
 const app=await memberHarness({saveFailure:true})
 await app.activate()
 assert.equal(app.elements.get('enable-notifications').hidden,true)
 assert.equal(app.elements.get('retry-setup').hidden,false)
 assert.equal(app.elements.get('setup-status').textContent,'Permissão ativa. Verificação pendente.')
 app.context.Notification.permission='denied'
 await app.listeners['window:focus']()
 assert.equal(app.elements.get('setup-status').textContent,'Seus avisos estão bloqueados.')
})

test('Closing and reopening the installed app restores the same device without another permission request',async()=>{
 const first=await memberHarness({permission:'default',existing:false,clan:'2'})
 await first.activate()
 await first.listeners['enable-notifications:click']()
 assert.equal(first.device.permissionRequests,1)
 const reopened=await memberHarness({device:first.device})
 await reopened.activate()
 assert.equal(reopened.elements.get('member-clan').value,'2')
 assert.equal(reopened.elements.get('setup-status').textContent,'Todos os avisos estão ativados.')
 assert.equal(reopened.device.subscribeCalls,1,'reuse the existing browser subscription')
 assert.equal(reopened.device.permissionRequests,1,'reopening cannot ask permission again')
})

test('Reopening with a lost clan preference recovers only the enrolled device clan',async()=>{
 for(const storageThrows of [false,true]){
  const first=await memberHarness({clan:'2'})
  await first.activate()
  first.device.localClan=''
  const reopened=await memberHarness({device:first.device,storageThrows})
  await reopened.activate()
  assert.equal(reopened.elements.get('member-clan').value,'2')
  assert.equal(reopened.elements.get('enable-notifications').hidden,true)
  assert.equal(reopened.elements.get('setup-status').textContent,'Todos os avisos estão ativados.')
  const restored=reopened.requests.find(item=>item.path==='/api/push/subscription/restore')
  assert.deepEqual(JSON.parse(restored.options.body),{subscription:{endpoint:'device-only'}})
  assert.equal(reopened.device.savedClan,'2','never move the enrollment to a guessed clan')
 }
})

test('A missing subscription on cold start is renewed with the granted permission',async()=>{
 const first=await memberHarness({clan:'2'})
 await first.activate()
 first.device.existing=false
 const reopened=await memberHarness({device:first.device})
 await reopened.activate()
 assert.equal(reopened.elements.get('setup-status').textContent,'Todos os avisos estão ativados.')
 assert.equal(reopened.device.subscribeCalls,1)
 assert.equal(reopened.device.permissionRequests,0)
 assert.equal(reopened.device.savedClan,'2')
})

test('A browser requiring a tap for renewal offers reconnection without requesting permission again',async()=>{
 const app=await memberHarness({existing:false,subscribeFailure:true})
 await app.activate()
 assert.equal(app.elements.get('enable-notifications').hidden,false)
 assert.equal(app.elements.get('enable-notifications').textContent,'Reconectar avisos')
 assert.equal(app.elements.get('setup-status').textContent,'Reconecte os avisos deste celular.')
 app.setSubscribeFailure(false)
 await app.listeners['enable-notifications:click']()
 assert.equal(app.elements.get('setup-status').textContent,'Todos os avisos estão ativados.')
 assert.equal(app.device.permissionRequests,0)
})

test('Reopening never subscribes with default or denied permission or an unknown clan',async()=>{
 for(const options of [{permission:'default'},{permission:'denied'},{clan:''}]){
  const app=await memberHarness({existing:false,...options})
  await app.activate()
  assert.equal(app.device.subscribeCalls,0)
  assert.equal(app.device.permissionRequests,0)
  assert.equal(app.elements.get('test-notification').hidden,true)
 }
})

test('The offline shell keeps the latest successful version and never caches APIs or error pages',async()=>{
 const listeners={},buckets=new Map([['imperio-members-v2',new Map([['/member.js',new Response('outdated')]])]])
 const navigated=[]
 let body='updated',status=200,offline=false,networkCache
 const caches={keys:async()=>[...buckets.keys()],delete:async key=>buckets.delete(key),open:async key=>{
  if(!buckets.has(key))buckets.set(key,new Map())
  const cache=buckets.get(key)
  return {put:async(path,response)=>cache.set(path,response),match:async path=>cache.get(path)?.clone()}
 }}
 const clients={claim:async()=>{},matchAll:async()=>['/app?clan=2#notice','/instalar','/','/dashboard','https://other.test/app'].map(path=>({url:new URL(path,'https://imperiobarbaro.com.br').href,navigate:url=>{navigated.push(url);return new Promise(()=>{})}}))}
 const context=vm.createContext({URL,caches,fetch:async(request,options)=>{networkCache=options.cache;if(offline)throw Error('Offline');return new Response(body,{status})},self:{location:{origin:'https://imperiobarbaro.com.br'},clients,addEventListener:(name,fn)=>listeners[name]=fn},importScripts(){}})
 vm.runInContext(await readFile(new URL('../public/pwa-sw.js',import.meta.url),'utf8'),context)
 let pending
 listeners.activate({waitUntil:p=>pending=p});await pending
 assert.equal(buckets.has('imperio-members-v2'),false)
 assert.deepEqual(navigated,['https://imperiobarbaro.com.br/app?clan=2#notice','https://imperiobarbaro.com.br/instalar'])
 // First install / an already upgraded cache must not reload the app again.
 listeners.activate({waitUntil:p=>pending=p});await pending
 assert.equal(navigated.length,2)
 const load=async(path,mode='cors')=>{
  pending=undefined
  listeners.fetch({request:{url:'https://imperiobarbaro.com.br'+path,method:'GET',mode},respondWith:p=>pending=p})
  return pending
 }
 assert.equal(await (await load('/member.js')).text(),'updated')
 assert.equal(networkCache,'no-store')
 body='newest'
 assert.equal(await (await load('/member.js')).text(),'newest')
 offline=true
 assert.equal(await (await load('/member.js')).text(),'newest')
 offline=false;status=503;body='server error'
 assert.equal(await (await load('/member.js')).text(),'newest')
 assert.equal(await load('/api/push/config'),undefined)
 assert.equal(await load('/api/contacts'),undefined)
 assert.equal(await load('/assets/dashboard.js'),undefined)
})
