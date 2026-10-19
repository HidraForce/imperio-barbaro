import test from 'node:test'
import assert from 'node:assert/strict'
import { createContactVault } from './contact-vault.js'
import { computed } from 'vue'
import { contactPhoneIndex } from './contact-numbers.js'

const json = (value, status = 200) => new Response(JSON.stringify(value), { status, headers: { 'Content-Type': 'application/json' } })
const session = () => ({ token: 'token-de-teste', expiresAt: new Date(Date.now() + 600000).toISOString() })

test('Contatos não são consultados antes do desbloqueio; senha é limpa e token segue no cabeçalho', async () => {
  const calls = []
  const vault = createContactVault(async (path, options) => {
    calls.push({ path, options })
    if (path.endsWith('/unlock')) return json(session())
    if (options.method === 'DELETE') return new Response(null, { status: 204 })
    return json([{ id: 1, playerName: 'Líder', phone: '5511999990000' }])
  })
  assert.equal(calls.length, 0)
  await vault.save()
  assert.equal(calls.length, 0)
  vault.password.value = 'senha-exclusiva-de-teste'
  const unlocking = vault.unlock()
  assert.equal(vault.password.value, '')
  await unlocking
  assert.equal(vault.unlocked.value, true)
  assert.equal(vault.contacts.value.length, 1)
  assert.equal(calls[1].options.headers.get('X-Contacts-Session'), 'token-de-teste')
  assert.equal(calls[0].options.cache, 'no-store')
  vault.form.value.realName = 'Dado privado'
  vault.lock()
  assert.deepEqual(vault.contacts.value, [])
  assert.equal(vault.form.value.realName, '')
  assert.equal(vault.unlocked.value, false)
  assert.equal(calls.at(-1).path, '/api/contacts/session')
})

test('Resposta tardia de contatos não restaura dados depois do bloqueio', async () => {
  let resolveContacts
  const vault = createContactVault(async (path, options) => {
    if (path.endsWith('/unlock')) return json(session())
    if (options.method === 'DELETE') return new Response(null, { status: 204 })
    return new Promise(resolve => { resolveContacts = resolve })
  })
  vault.password.value = 'senha'
  const unlocking = vault.unlock()
  while (!resolveContacts) await new Promise(resolve => setImmediate(resolve))
  vault.lock()
  resolveContacts(json([{ id: 1, playerName: 'Privado' }]))
  await unlocking
  assert.equal(vault.unlocked.value, false)
  assert.deepEqual(vault.contacts.value, [])
})

test('Desbloqueio recebido após sair da área é revogado', async () => {
  let resolveUnlock
  const calls = []
  const vault = createContactVault(async (path, options) => {
    calls.push(path)
    if (options.method === 'DELETE') return new Response(null, { status: 204 })
    return new Promise(resolve => { resolveUnlock = resolve })
  })
  vault.password.value = 'senha'
  const unlocking = vault.unlock()
  vault.lock()
  resolveUnlock(json(session()))
  await unlocking
  assert.deepEqual(calls, ['/api/contacts/unlock', '/api/contacts/session'])
  assert.equal(vault.unlocked.value, false)
})

test('Sessão rejeitada pelo servidor limpa os contatos e o formulário', async () => {
  let expired = false
  const vault = createContactVault(async (path, options) => {
    if (path.endsWith('/unlock')) return json(session())
    if (path.endsWith('/session')) return new Response(null, { status: 204 })
    return expired ? json({ error: 'Contatos bloqueados.' }, 423) : json([{ id: 1 }])
  })
  vault.password.value = 'senha'
  await vault.unlock()
  expired = true
  await vault.remove(1)
  assert.equal(vault.unlocked.value, false)
  assert.deepEqual(vault.contacts.value, [])
  assert.equal(vault.error.value, 'Contatos bloqueados.')
})

test('O prazo absoluto bloqueia a tela e revoga a sessão automaticamente', async t => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const calls = []
  const vault = createContactVault(async (path, options) => {
    calls.push(path)
    if (path.endsWith('/unlock')) return json(session())
    if (options.method === 'DELETE') return new Response(null, { status: 204 })
    return json([{ id: 1 }])
  })
  vault.password.value = 'senha'
  await vault.unlock()
  t.mock.timers.tick(600001)
  assert.equal(vault.unlocked.value, false)
  assert.deepEqual(vault.contacts.value, [])
  assert.match(vault.error.value, /expirou/)
  assert.equal(calls.at(-1), '/api/contacts/session')
})

test('Edição copia as contas e salva uma pessoa com contas nos dois clãs', async () => {
  const original = { id: 7, realName: 'Bruno', phone: '', notes: '', accounts: [{ playerTag: '#P0Y8LQ', playerName: 'Principal', clanId: '1' }] }
  let saved
  const vault = createContactVault(async (path, options) => {
    if (path.endsWith('/unlock')) return json(session())
    if (path.endsWith('/session')) return new Response(null, { status: 204 })
    if (options.method === 'POST') { saved = JSON.parse(options.body); return new Response(null, { status: 204 }) }
    return json([original])
  })
  vault.password.value = 'senha'
  await vault.unlock()
  vault.edit(original)
  vault.form.value.accounts[0].playerName = 'Principal editada'
  assert.equal(original.accounts[0].playerName, 'Principal')
  vault.addAccount()
  Object.assign(vault.form.value.accounts[1], { playerTag: '#QY8L0P', playerName: 'Secundária', clanId: '2' })
  await vault.save()
  assert.equal(saved.id, 7)
  assert.deepEqual(saved.accounts.map(a => a.clanId), ['1', '2'])
  assert.equal(vault.showForm.value, false)
  vault.lock()
})

test('Shared number access expires and clears message phones; late refresh cannot restore them',async t=>{
  t.mock.timers.enable({apis:['setTimeout']})
  let refreshing=false,resolveRefresh
  const person={phone:'5511999991234',accounts:[{playerTag:'#PYQ'}]}
  const vault=createContactVault(async(path,options)=>{
    if(path.endsWith('/unlock'))return json(session())
    if(options.method==='DELETE')return new Response(null,{status:204})
    if(refreshing)return new Promise(resolve=>{resolveRefresh=resolve})
    return json([person])
  })
  const phones=computed(()=>vault.unlocked.value?contactPhoneIndex(vault.contacts.value):{})
  assert.deepEqual(phones.value,{})
  vault.password.value='test'
  await vault.unlock()
  assert.equal(phones.value['#PYQ'],'5511999991234')
  refreshing=true
  const pending=vault.refresh()
  while(!resolveRefresh)await new Promise(resolve=>setImmediate(resolve))
  t.mock.timers.tick(600001)
  assert.deepEqual(phones.value,{})
  resolveRefresh(json([person]))
  await pending
  assert.deepEqual(phones.value,{})
  assert.deepEqual(vault.contacts.value,[])
})

test('Expired contact session during refresh removes phones rather than retaining stale data',async()=>{
  let expired=false
  const vault=createContactVault(async(path,options)=>{
    if(path.endsWith('/unlock'))return json(session())
    if(options.method==='DELETE')return new Response(null,{status:204})
    return expired?json({error:'Expired'},423):json([{phone:'5511999991234',accounts:[{playerTag:'#PYQ'}]}])
  })
  vault.password.value='test';await vault.unlock();expired=true;await vault.refresh()
  assert.equal(vault.unlocked.value,false)
  assert.deepEqual(vault.contacts.value,[])
})
