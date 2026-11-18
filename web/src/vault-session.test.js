import test from 'node:test'
import assert from 'node:assert/strict'
import { createVaultSession } from './vault-session.js'
import { createScopedVault } from './announcement-vault.js'
import { createContactVault } from './contact-vault.js'

function mockSessionStorage(t) {
  const original = Object.getOwnPropertyDescriptor(globalThis, 'sessionStorage')
  const entries = new Map()
  Object.defineProperty(globalThis, 'sessionStorage', {
    configurable: true,
    value: {
      getItem: key => entries.get(key) ?? null,
      setItem: (key, value) => entries.set(key, value),
      removeItem: key => entries.delete(key),
    },
  })
  t.after(() => {
    if (original) Object.defineProperty(globalThis, 'sessionStorage', original)
    else delete globalThis.sessionStorage
  })
  return entries
}

test('Returning after a refresh keeps the original session deadline', async t => {
  const entries = mockSessionStorage(t)
  t.mock.timers.enable({ apis: ['Date', 'setTimeout'], now: 1800000000000 })
  const calls = []
  const api = async (path, options) => {
    calls.push({ path, options })
    if (path.endsWith('/unlock')) return { token: 'clan-session', expiresAt: new Date(Date.now() + 600000).toISOString() }
    return {}
  }
  const first = createScopedVault('/api/announcements/clans/1', 'X-Clan-Announcements-Session', api)
  first.password.value = 'private-password'
  await first.unlock()
  const saved = [...entries.values()]
  assert.deepEqual(JSON.parse(saved[0]), { token: 'clan-session', expiresAt: Date.now() + 600000 })
  assert.equal(saved[0].includes('private-password'), false)
  first.dispose()
  assert.equal(calls.length, 1)

  t.mock.timers.tick(120000)
  const restored = createScopedVault('/api/announcements/clans/1', 'X-Clan-Announcements-Session', api)
  t.after(() => restored.lock())
  assert.equal(restored.unlocked.value, true)
  await restored.request('/api/announcements?clan=1')
  assert.equal(calls.at(-1).options.headers['X-Clan-Announcements-Session'], 'clan-session')
  assert.deepEqual([...entries.values()], saved)
  t.mock.timers.tick(479999)
  assert.equal(restored.unlocked.value, true)
  t.mock.timers.tick(1)
  assert.equal(restored.unlocked.value, false)
  assert.equal(entries.size, 0)
  assert.equal(calls.at(-1).options.method, 'DELETE')
})

test('All clan, notification, and editor unlocks remain isolated across refreshes', async t => {
  const entries = mockSessionStorage(t)
  const paths = ['announcements/clans/1', 'announcements/clans/2', 'announcements/clans/3', 'announcements/custom', 'announcements/timer', 'leadership', 'rules']
  const api = async path => path.endsWith('/unlock') ? { token: path, expiresAt: new Date(Date.now() + 600000).toISOString() } : {}
  const restored = []
  t.after(() => restored.forEach(vault => vault.lock()))
  for (const path of paths) {
    const vault = createScopedVault('/api/' + path, 'X-Test-Session', api)
    await vault.unlock()
    vault.dispose()
    restored.push(createScopedVault('/api/' + path, 'X-Test-Session', api))
  }
  assert.equal(entries.size, paths.length)
  assert.ok(restored.every(vault => vault.unlocked.value))
  restored[0].lock()
  assert.equal(entries.size, paths.length - 1)
  const locked = createScopedVault('/api/announcements/clans/1', 'X-Test-Session', api)
  t.after(() => locked.dispose())
  assert.equal(locked.unlocked.value, false)
  assert.ok(restored.slice(1).every(vault => vault.unlocked.value))
})

test('Returning from a suspended page checks expiry before a timer fires', async t => {
  const entries = mockSessionStorage(t)
  t.mock.timers.enable({ apis: ['Date', 'setTimeout'], now: 1800000000000 })
  const calls = []
  const api = async (path, options) => {
    calls.push({ path, options })
    return { token: 'session', expiresAt: new Date(Date.now() + 600000).toISOString() }
  }
  const vault = createScopedVault('/api/rules', 'X-Editor-Session', api)
  t.after(() => vault.lock())
  await vault.unlock()
  t.mock.timers.setTime(Date.now() + 600001)
  vault.resume()
  assert.equal(vault.unlocked.value, false)
  assert.equal(entries.size, 0)
  await assert.rejects(vault.request('/api/rules'), /Desbloqueie/)
  assert.ok(!calls.some(call => call.path === '/api/rules'))
})

test('The server rejecting a cached session removes it from the browser', async t => {
  const entries = mockSessionStorage(t)
  createVaultSession('/api/leadership').save({ token: 'revoked', expiresAt: new Date(Date.now() + 600000).toISOString() })
  const api = async path => {
    if (path.endsWith('/session')) return null
    throw Object.assign(Error('Sessão encerrada'), { status: 423 })
  }
  const vault = createScopedVault('/api/leadership', 'X-Editor-Session', api)
  t.after(() => vault.lock())
  assert.equal(vault.unlocked.value, true)
  await assert.rejects(vault.request('/api/leadership'), /Sessão encerrada/)
  assert.equal(vault.unlocked.value, false)
  assert.equal(entries.size, 0)
})

test('A late unlock after leaving the component is revoked without caching', async t => {
  const entries = mockSessionStorage(t)
  const calls = []
  let finish
  const api = (path, options) => {
    calls.push({ path, options })
    return path.endsWith('/unlock') ? new Promise(resolve => { finish = resolve }) : Promise.resolve(null)
  }
  const vault = createScopedVault('/api/announcements/custom', 'X-Notifications-Session', api)
  const pending = vault.unlock()
  vault.dispose()
  finish({ token: 'late-session', expiresAt: new Date(Date.now() + 600000).toISOString() })
  await pending
  assert.equal(entries.size, 0)
  assert.equal(vault.unlocked.value, false)
  assert.equal(calls.at(-1).options.method, 'DELETE')
})

test('Restored contact access reloads contacts without caching private form data', async t => {
  const entries = mockSessionStorage(t)
  const calls = []
  const fetcher = async (path, options) => {
    calls.push({ path, options })
    if (path.endsWith('/unlock')) return Response.json({ token: 'contacts-session', expiresAt: new Date(Date.now() + 600000).toISOString() })
    if (options.method === 'DELETE') return new Response(null, { status: 204 })
    return Response.json([{ id: 1, realName: 'Pessoa', phone: '5511999990000', accounts: [] }])
  }
  const first = createContactVault(fetcher)
  first.password.value = 'contacts-password'
  await first.unlock()
  first.form.value.realName = 'Rascunho privado'
  first.dispose()
  assert.ok(!calls.some(call => call.options.method === 'DELETE'))
  const restored = createContactVault(fetcher)
  t.after(() => restored.lock())
  await restored.resume()
  assert.equal(restored.unlocked.value, true)
  assert.equal(restored.contacts.value[0].realName, 'Pessoa')
  assert.equal(calls.filter(call => call.path.endsWith('/unlock')).length, 1)
  assert.equal(calls.at(-1).options.headers.get('X-Contacts-Session'), 'contacts-session')
  const cached = [...entries.values()].join('')
  assert.equal(cached.includes('contacts-password'), false)
  assert.equal(cached.includes('5511999990000'), false)
  assert.equal(cached.includes('Rascunho privado'), false)
})

test('Expired and malformed cached sessions are discarded and lifetimes are capped', t => {
  const entries = mockSessionStorage(t)
  const cache = createVaultSession('/api/rules')
  const key = 'clanboard:vault:/api/rules'
  for (const value of ['invalid JSON', JSON.stringify({ token: 'expired', expiresAt: Date.now() - 1 }), JSON.stringify({ token: 'invalid', expiresAt: 'tomorrow' })]) {
    entries.set(key, value)
    assert.equal(cache.restore(), null)
    assert.equal(entries.size, 0)
  }
  const session = cache.save({ token: 'bounded', expiresAt: new Date(Date.now() + 3600000).toISOString() })
  assert.ok(session.expiresAt - Date.now() <= 600000)
  assert.ok(cache.restore())
})

test('Blocked browser storage still allows an in-memory unlock', async t => {
  mockSessionStorage(t)
  Object.defineProperty(globalThis, 'sessionStorage', { configurable: true, get() { throw Error('Storage unavailable') } })
  const api = async () => ({ token: 'memory-session', expiresAt: new Date(Date.now() + 600000).toISOString() })
  const vault = createScopedVault('/api/rules', 'X-Editor-Session', api)
  t.after(() => vault.lock())
  await vault.unlock()
  assert.equal(vault.unlocked.value, true)
  await vault.request('/api/rules')
  vault.lock()
  assert.equal(vault.unlocked.value, false)
})
