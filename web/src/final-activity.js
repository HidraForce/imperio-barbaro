export function clashTime(value) {
  return Date.parse(String(value || '').replace(/^(\d{4})(\d{2})(\d{2})T(\d{2})(\d{2})(\d{2})\.000Z$/, '$1-$2-$3T$4:$5:$6Z'))
}

export function confirmedEnd(event, state, now = Date.now()) {
  const start = clashTime(event?.startTime), end = clashTime(event?.endTime)
  return event?.state === state && Number.isFinite(start) && Number.isFinite(end) && start < end && end <= now
}

export function completeWarAttacks(war, side, limit) {
  if (!Number.isInteger(war?.teamSize) || war.teamSize < 1 || side?.members?.length !== war.teamSize || !Number.isInteger(side.attacks)) return false
  const tags = new Set()
  let total = 0
  for (const member of side.members) {
    if (!member.tag || tags.has(member.tag) || (member.attacks != null && !Array.isArray(member.attacks))) return false
    tags.add(member.tag)
    const count = member.attacks?.length || 0
    if (count > limit) return false
    total += count
  }
  return total === side.attacks
}

export function completeRaidAttacks(season) {
  if (!Number.isInteger(season?.totalAttacks) || season.totalAttacks < 0 || (season.members != null && !Array.isArray(season.members))) return false
  const tags = new Set()
  let total = 0
  for (const member of season.members || []) {
    if (!member.tag || tags.has(member.tag) || !Number.isInteger(member.attacks) || member.attacks < 0 || member.attacks > 6) return false
    tags.add(member.tag)
    total += member.attacks
  }
  return total === season.totalAttacks
}
