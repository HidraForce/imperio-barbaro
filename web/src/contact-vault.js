import { ref } from 'vue'
import { createVaultSession } from './vault-session.js'

const emptyAccount = () => ({ playerTag: '', playerName: '', clanId: '' })
const emptyForm = () => ({ id: 0, realName: '', phone: '', notes: '', accounts: [emptyAccount()] })

export function createContactVault(fetcher = (...args) => fetch(...args)) {
  const contacts = ref([]), form = ref(emptyForm()), password = ref(''), unlocked = ref(false), busy = ref(false), error = ref(''), showForm = ref(false)
  const sessionCache = createVaultSession('/api/contacts')
  let token = '', expiresAt = 0, generation = 0, timer, disposed = false

  async function request(path, options = {}, session = token) {
    if (session && session === token && Date.now() >= expiresAt) {
      lock('O acesso aos contatos expirou. Desbloqueie novamente.')
      throw Error('O acesso aos contatos expirou. Desbloqueie novamente.')
    }
    const headers = new Headers(options.headers)
    headers.set('X-Clanboard-Request', '1')
    if (session) headers.set('X-Contacts-Session', session)
    const response = await fetcher(path, { ...options, headers, credentials: 'same-origin', cache: 'no-store' })
    if (!response.ok) {
      const body = await response.json().catch(() => ({}))
      const failure = Error(body.error || 'Não foi possível acessar os contatos.')
      failure.status = response.status
      throw failure
    }
    return response.status === 204 ? null : response.json()
  }

  function revoke(session) {
    if (session) void request('/api/contacts/session', { method: 'DELETE', keepalive: true }, session).catch(() => {})
  }

  function reset(message = '') {
    generation++
    clearTimeout(timer)
    token = ''
    expiresAt = 0
    unlocked.value = false
    contacts.value = []
    form.value = emptyForm()
    password.value = ''
    showForm.value = false
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
    timer = setTimeout(() => lock('O acesso aos contatos expirou. Desbloqueie novamente.'), expiresAt - Date.now())
  }

  async function resume() {
    if (disposed) return
    if (token) {
      if (Date.now() >= expiresAt) lock('O acesso aos contatos expirou. Desbloqueie novamente.')
      return
    }
    const session = sessionCache.restore()
    if (!session) return
    useSession(session)
    await refresh()
  }

  function dispose() {
    disposed = true
    reset()
  }

  async function unlock() {
    if (disposed || busy.value || unlocked.value) return
    const current = ++generation
    busy.value = true
    error.value = ''
    const secret = password.value
    password.value = ''
    try {
      const result = await request('/api/contacts/unlock', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ password: secret }) })
      if (current !== generation) { revoke(result.token); return }
      try { useSession(sessionCache.save(result)) }
      catch (failure) { revoke(result.token); throw failure }
      const items = await request('/api/contacts')
      if (current === generation) contacts.value = items
    } catch (failure) {
      if (current === generation) lock(failure.message)
    } finally { if (current === generation) busy.value = false }
  }

  async function change(path, options) {
    if (!unlocked.value || busy.value) return
    const current = generation
    busy.value = true
    error.value = ''
    try {
      await request(path, options)
      if (current !== generation) return
      const items = await request('/api/contacts')
      if (current !== generation) return
      contacts.value = items
      form.value = emptyForm()
      showForm.value = false
    } catch (failure) {
      if (current === generation) {
        if ([401, 423, 503].includes(failure.status)) lock(failure.message)
        else error.value = failure.message
      }
    } finally { if (current === generation) busy.value = false }
  }

  async function refresh() {
    if (!unlocked.value || busy.value) return
    const current = generation
    busy.value = true
    try {
      const items = await request('/api/contacts')
      if (current === generation) contacts.value = items
    } catch (failure) {
      if (current === generation) lock(failure.message)
    } finally { if (current === generation) busy.value = false }
  }

  function startNew() { if (!unlocked.value||busy.value) return; form.value=emptyForm();showForm.value=true;error.value='' }
  function edit(contact) { if (!unlocked.value||busy.value) return; form.value={id:contact.id,realName:contact.realName,phone:contact.phone,notes:contact.notes,accounts:contact.accounts.map(account=>({...account}))};showForm.value=true;error.value='' }
  function addAccount() { if (form.value.accounts.length<20) form.value.accounts.push(emptyAccount()) }
  function removeAccount(index) { if(form.value.accounts.length>1) form.value.accounts.splice(index,1) }
  return { contacts, form, password, unlocked, busy, error, showForm, unlock, lock, refresh, resume, dispose, startNew, edit, addAccount, removeAccount, request,
    merge: (targetId,sourceId) => change('/api/contacts/merge', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({targetId:Number(targetId),sourceId:Number(sourceId)})}),
    save: () => change('/api/contacts', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(form.value) }),
    remove: id => change('/api/contacts/' + id, { method: 'DELETE' }),
  }
}
