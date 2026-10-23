import { ref } from 'vue'
import { adminAPI } from './admin-api.js'

export function createAnnouncementVault(scope, header, api = adminAPI) {
  const password = ref(''), unlocked = ref(false), busy = ref(false), error = ref('')
  let token = '', generation = 0, timer
  const path = '/api/announcements/' + scope
  function revoke(session) {
    if (session) void api(path + '/session', { method: 'DELETE', keepalive: true, headers: { [header]: session } }).catch(() => {})
  }
  function lock(message = '') {
    const previous = token
    generation++
    clearTimeout(timer)
    token = ''
    password.value = ''
    unlocked.value = false
    busy.value = false
    error.value = message
    revoke(previous)
  }
  async function unlock() {
    if (busy.value || unlocked.value) return
    const current = ++generation, secret = password.value
    password.value = ''
    busy.value = true
    error.value = ''
    try {
      const result = await api(path + '/unlock', { method: 'POST', body: JSON.stringify({ password: secret }) })
      if (current !== generation) { revoke(result.token); return }
      const lifetime = Date.parse(result.expiresAt) - Date.now()
      if (!result.token || !Number.isFinite(lifetime) || lifetime <= 0) { revoke(result.token); throw Error('O desbloqueio expirou. Tente novamente.') }
      token = result.token
      unlocked.value = true
      timer = setTimeout(() => lock('O acesso expirou. Desbloqueie novamente.'), Math.min(lifetime, 10 * 60 * 1000))
    } catch (failure) {
      if (current === generation) lock(failure.message)
    } finally { if (current === generation) busy.value = false }
  }
  async function request(url, options = {}) {
    if (!token) throw Error('Desbloqueie este acesso para continuar.')
    try { return await api(url, { ...options, headers: { ...options.headers, [header]: token } }) }
    catch (failure) {
      if ([401, 423].includes(failure.status)) lock(failure.message)
      throw failure
    }
  }
  return { password, unlocked, busy, error, lock, unlock, request }
}
