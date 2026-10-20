const roleRank = { leader: 4, coLeader: 3, admin: 2, member: 1 }
const trophies = member => Number.isFinite(Number(member.trophies)) ? Number(member.trophies) : -1

export function sortMembers(members = [], field = 'trophies', direction = 'desc') {
  const order = direction === 'asc' ? 1 : -1
  return [...members].sort((a, b) => {
    const primary = field === 'role' ? (roleRank[a.role] || 0) - (roleRank[b.role] || 0) : trophies(a) - trophies(b)
    return order * primary || (field === 'role' ? trophies(b) - trophies(a) : 0) || String(a.name || '').localeCompare(String(b.name || ''), 'pt-BR') || String(a.tag || '').localeCompare(String(b.tag || ''))
  })
}
