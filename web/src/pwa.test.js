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

async function memberHarness({permission='granted',existing=true,saveFailure=false}={}){
 let readFailure=false
 const elements=new Map(),listeners={},swListeners={}
 function element(id){if(!elements.has(id))elements.set(id,{value:'',hidden:false,disabled:false,textContent:'',classList:{add(){},toggle(){},contains(){return false}},addEventListener:(name,fn)=>listeners[id+':'+name]=fn,replaceChildren(){}});return elements.get(id)}
 let activate,registered
 const ready=new Promise(resolve=>activate=resolve),registeredPromise=new Promise(resolve=>registered=resolve)
 const requests=[],subscription={toJSON:()=>({endpoint:'device-only'})}
 const activeRegistration={pushManager:{getSubscription:async()=>{if(readFailure)throw Error('Não foi possível ler a inscrição agora.');return existing?subscription:null},subscribe:async()=>{existing=true;return subscription}}}
 const context=vm.createContext({memberSetup,memberClan,memberAppURL,URL,URLSearchParams,Uint8Array,atob,
  isSecureContext:true,PushManager:function(){},Notification:{permission,requestPermission:async()=>{context.Notification.permission='granted';return 'granted'}},
  navigator:{userAgent:'Android',platform:'Linux',serviceWorker:{register:async()=>{registered();return {pushManager:{getSubscription(){throw Error('Inactive registration used')}}}},ready,addEventListener:(name,fn)=>swListeners[name]=fn}},
  document:{hidden:false,body:{dataset:{page:'install'},classList:{toggle(){}}},getElementById:element,createElement:()=>element(Symbol()),addEventListener:(name,fn)=>listeners['document:'+name]=fn},
  matchMedia:()=>({matches:true}),localStorage:{getItem:()=> '1',setItem(){}},location:{search:'',pathname:'/instalar',hash:''},history:{replaceState(){}},
  setInterval(){return 1},fetch:async(path,options)=>{requests.push({path,options});if(path==='/api/push/subscription'&&saveFailure){if(saveFailure==='offline')throw Error('Sem conexão.');return {ok:false,json:async()=>({error:'O servidor está indisponível.'})}}return {ok:true,json:async()=>path==='/api/push/config'?{publicKey:'AA'}:{accepted:true}}},addEventListener:(name,fn)=>listeners['window:'+name]=fn,
 })
 context.window=context
 const source=await readFile(new URL('../public/member.js',import.meta.url),'utf8')
 vm.runInContext(source.replace(/^import .*\n/,''),context)
 await registeredPromise
 return {elements,listeners,requests,context,setSaveFailure:value=>saveFailure=value,setReadFailure:value=>readFailure=value,setExisting:value=>existing=value,activate:async()=>{activate(activeRegistration);await vm.runInContext('bootPromise',context)}}
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
 // A confirmed absent subscription really does require another activation.
 app.setExisting(false)
 await app.listeners['window:focus']()
 assert.equal(app.elements.get('enable-notifications').hidden,false)
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
