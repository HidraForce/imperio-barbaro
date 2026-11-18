import { ref } from 'vue'
import { adminAPI } from './admin-api.js'
import { createVaultSession } from './vault-session.js'

export function createAnnouncementVault(scope, header, api = adminAPI) {
  return createScopedVault('/api/announcements/' + scope, header, api)
}

export function createScopedVault(path, header, api = adminAPI) {
  const password = ref(''), unlocked = ref(false), busy = ref(false), error = ref('')
  const sessionCache = createVaultSession(path)
  let token = '', expiresAt = 0, generation = 0, timer, disposed = false
  function revoke(session) {
    if (session) void api(path + '/session', { method: 'DELETE', keepalive: true, headers: { [header]: session } }).catch(() => {})
  }
  function reset(message = '') {
    generation++
    clearTimeout(timer)
    token = ''
    expiresAt = 0
    password.value = ''
    unlocked.value = false
    busy.value = false
    error.value = message
  }
  function lock(message = '') {
    const previous = token
    sessionCache.clear()
    reset(message)
    revoke(previous)
  }
  function useSession(session) {
    token = session.token
    expiresAt = session.expiresAt
    unlocked.value = true
    timer = setTimeout(() => lock('O acesso expirou. Desbloqueie novamente.'), expiresAt - Date.now())
  }
  function resume() {
    if (disposed) return
    if (token) {
      if (Date.now() >= expiresAt) lock('O acesso expirou. Desbloqueie novamente.')
      return
    }
    const session = sessionCache.restore()
    if (session) useSession(session)
  }
  function dispose() {
    disposed = true
    reset()
  }
  async function unlock() {
    if (disposed || busy.value || unlocked.value) return
    const current = ++generation, secret = password.value
    password.value = ''
    busy.value = true
    error.value = ''
    try {
      const result = await api(path + '/unlock', { method: 'POST', body: JSON.stringify({ password: secret }) })
      if (current !== generation) { revoke(result.token); return }
      try { useSession(sessionCache.save(result)) }
      catch (failure) { revoke(result.token); throw failure }
    } catch (failure) {
      if (current === generation) lock(failure.message)
    } finally { if (current === generation) busy.value = false }
  }
  async function request(url, options = {}, send = api) {
    resume()
    if (!token) throw Error('Desbloqueie este acesso para continuar.')
    const current = generation
    try {
      const result = await send(url, { ...options, headers: { ...options.headers, [header]: token } })
      if (current !== generation) throw Error('O acesso foi encerrado. Desbloqueie novamente.')
      return result
    }
    catch (failure) {
      if (current === generation && [401, 423].includes(failure.status)) lock(failure.message)
      throw failure
    }
  }
  resume()
  return { password, unlocked, busy, error, lock, unlock, request, resume, dispose }
}
