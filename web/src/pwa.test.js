import test from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import vm from 'node:vm'
import { memberSetup } from '../public/pwa-state.js'

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
 listeners.notificationclick({notification:{close(){},data:{url:'https://evil.test/'}},waitUntil:p=>pending=p});await pending
 assert.equal(opened[0],'https://imperiobarbaro.com.br/app')
 listeners.notificationclick({notification:{close(){},data:{url:'/app?clan=2#abc'}},waitUntil:p=>pending=p});await pending
 assert.equal(opened[1],'https://imperiobarbaro.com.br/app?clan=2#abc')
 let intercepted=false
 listeners.fetch({request:{url:'https://imperiobarbaro.com.br/api/contacts',method:'GET'},respondWith(){intercepted=true}})
 assert.equal(intercepted,false,'private APIs must not be cached')
})
