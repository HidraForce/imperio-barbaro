import test from 'node:test'
import assert from 'node:assert/strict'
import { createAnnouncementVault } from './announcement-vault.js'
import { adminAPI } from './admin-api.js'

test('Clan sessions combine with the existing custom and timer sessions', async () => {
 const calls=[]
 const api=async(path,options)=>{calls.push({path,options});return {token:path.split('/').at(-2)+'-token',expiresAt:new Date(Date.now()+60000).toISOString()}}
 const clan=createAnnouncementVault('clans/3','X-Clan-Announcements-Session',api)
 const custom=createAnnouncementVault('custom','X-Notifications-Session',api)
 const timer=createAnnouncementVault('timer','X-Super-Notifications-Session',api)
 try {
  custom.password.value='custom-secret';await custom.unlock()
  assert.equal(custom.password.value,'')
  assert.equal(timer.unlocked.value,false)
  await clan.unlock()
  assert.equal(calls.at(-1).path,'/api/announcements/clans/3/unlock')
  timer.password.value='timer-secret';await timer.unlock()
  await custom.request('/api/announcements/custom',{method:'POST'},clan.request)
  assert.deepEqual(calls.at(-1).options.headers,{'X-Notifications-Session':'custom-token','X-Clan-Announcements-Session':'3-token'})
  await timer.request('/api/announcements/timer/reset',{method:'POST'},clan.request)
  assert.deepEqual(calls.at(-1).options.headers,{'X-Super-Notifications-Session':'timer-token','X-Clan-Announcements-Session':'3-token'})
  custom.lock()
  assert.equal(timer.unlocked.value,true)
  assert.ok(!calls.some(call=>call.path.startsWith('/api/contacts')))
 }finally{clan.lock();custom.lock();timer.lock()}
})

test('Hiding while an unlock is in flight revokes the late session',async()=>{
 let finish
 const calls=[]
 const api=(path,options)=>{calls.push({path,options});return path.endsWith('/unlock')?new Promise(resolve=>finish=resolve):Promise.resolve(null)}
 const vault=createAnnouncementVault('timer','X-Super-Notifications-Session',api)
 vault.password.value='secret'
 const pending=vault.unlock();vault.lock()
 finish({token:'late-session',expiresAt:new Date(Date.now()+60000).toISOString()})
 await pending
 assert.equal(vault.unlocked.value,false)
 assert.equal(calls.at(-1).path,'/api/announcements/timer/session')
 assert.equal(calls.at(-1).options.headers['X-Super-Notifications-Session'],'late-session')
 await assert.rejects(vault.request('/api/announcements/timer/reset'),/Desbloqueie/)
})

test('Expired sessions cannot remain unlocked after a protected request',async()=>{
 const api=async(path)=>{if(path.endsWith('/unlock'))return {token:'session',expiresAt:new Date(Date.now()+60000).toISOString()};if(path.endsWith('/session'))return null;throw Object.assign(Error('Expirou'),{status:423})}
 const vault=createAnnouncementVault('custom','X-Notifications-Session',api)
 await vault.unlock()
 await assert.rejects(vault.request('/api/announcements/custom'),/Expirou/)
 assert.equal(vault.unlocked.value,false)
 assert.equal(vault.error.value,'Expirou')
})

test('Admin API preserves capability headers along with JSON and CSRF protection',async()=>{
 const original=globalThis.fetch
 let captured
 globalThis.fetch=async(path,options)=>{captured=options;return new Response('{}')}
 try {
  await adminAPI('/api/announcements/custom',{method:'POST',headers:{'X-Notifications-Session':'token'}})
  assert.equal(captured.headers['X-Notifications-Session'],'token')
  assert.equal(captured.headers['X-Clanboard-Request'],'1')
  assert.equal(captured.headers['Content-Type'],'application/json')
 }finally{globalThis.fetch=original}
})
