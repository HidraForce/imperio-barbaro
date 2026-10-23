import { ref } from 'vue'

const emptyAccount = () => ({ playerTag: '', playerName: '', clanId: '' })
const emptyForm = () => ({ id: 0, realName: '', phone: '', notes: '', accounts: [emptyAccount()] })

export function createContactVault(fetcher = (...args) => fetch(...args)) {
  const contacts = ref([]), form = ref(emptyForm()), password = ref(''), unlocked = ref(false), busy = ref(false), error = ref(''), showForm = ref(false)
  let token = '', generation = 0, timer

  async function request(path, options = {}, session = token) {
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

  function lock(message = '') {
    const previous = token
    generation++
    clearTimeout(timer)
    token = ''
    unlocked.value = false
    contacts.value = []
    form.value = emptyForm()
    password.value = ''
    showForm.value = false
    busy.value = false
    error.value = message
    revoke(previous)
  }

  async function unlock() {
    if (busy.value || unlocked.value) return
    const current = ++generation
    busy.value = true
    error.value = ''
    const secret = password.value
    password.value = ''
    try {
      const result = await request('/api/contacts/unlock', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ password: secret }) })
      if (current !== generation) { revoke(result.token); return }
      const lifetime = Date.parse(result.expiresAt) - Date.now()
      if (!result.token || !Number.isFinite(lifetime) || lifetime <= 0) { revoke(result.token); throw Error('O desbloqueio expirou. Tente novamente.') }
      token = result.token
      unlocked.value = true
      timer = setTimeout(() => lock('O acesso aos contatos expirou. Desbloqueie novamente.'), Math.min(lifetime, 10 * 60 * 1000))
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
  return { contacts, form, password, unlocked, busy, error, showForm, unlock, lock, refresh, startNew, edit, addAccount, removeAccount, request,
    merge: (targetId,sourceId) => change('/api/contacts/merge', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({targetId:Number(targetId),sourceId:Number(sourceId)})}),
    save: () => change('/api/contacts', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(form.value) }),
    remove: id => change('/api/contacts/' + id, { method: 'DELETE' }),
  }
}
