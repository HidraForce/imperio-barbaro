import { adminAPI } from './admin-api.js'

const tagKey = value => String(value || '').replace(/^#/, '').trim().toUpperCase()

export async function loadContactMembers(clans, cached = {}, fetcher = adminAPI) {
  const configured = clans.filter(clan => clan.configured)
  const results = await Promise.allSettled(configured.map(async clan => {
    const members = cached[clan.id] || (await fetcher('/api/clan?clan=' + encodeURIComponent(clan.id))).memberList
    if (!Array.isArray(members)) throw Error('Lista de membros indisponível.')
    return [clan.id, members]
  }))
  return {
    lists: Object.fromEntries(results.flatMap(result => result.status === 'fulfilled' ? [result.value] : [])),
    failed: configured.filter((clan, index) => results[index].status === 'rejected').map(clan => clan.id),
  }
}

export function contactMemberOptions(clans, lists) {
  const members = new Map()
  for (const clan of clans) {
    for (const member of lists[clan.id] || []) {
      const key = tagKey(member.tag)
      if (key && member.name && !members.has(key)) members.set(key, {playerTag:'#'+key, playerName:member.name, clanId:clan.id})
    }
  }
  return [...members.values()].sort((a, b) => a.clanId.localeCompare(b.clanId) || a.playerName.localeCompare(b.playerName, 'pt-BR'))
}

export function accountMemberOptions(members, contacts, form, account) {
  const selected = tagKey(account.playerTag)
  const options = [...members]
  if (selected && !options.some(member => tagKey(member.playerTag) === selected)) options.push({...account, playerTag:'#'+selected, saved:true})
  return options.map(member => {
    const key = tagKey(member.playerTag)
    const other = contacts.find(contact => contact.id !== form.id && contact.accounts?.some(item => tagKey(item.playerTag) === key))
    const duplicate = form.accounts.some(item => item !== account && tagKey(item.playerTag) === key)
    return {...member, disabled:!!other || duplicate, unavailable:other ? 'Já vinculada a outro contato' : duplicate ? 'Já selecionada nesta pessoa' : ''}
  })
}

export function selectContactMember(account, tag, members) {
  const selected = members.find(member => tagKey(member.playerTag) === tagKey(tag))
  if (!selected || selected.disabled) return false
  Object.assign(account, {playerTag:selected.playerTag, playerName:selected.playerName, clanId:selected.clanId})
  return true
}
