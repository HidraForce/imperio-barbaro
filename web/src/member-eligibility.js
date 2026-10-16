export function clashDate(value) {
  return Date.parse(String(value || '').replace(/^(\d{4})(\d{2})(\d{2})T(\d{2})(\d{2})(\d{2})\.000Z$/, '$1-$2-$3T$4:$5:$6Z'))
}

export function entryEligibility(member, event) {
  const history = member?.membership
  if (!history?.detectedJoin) return ''
  const end = clashDate(event?.endTime)
  if (Date.parse(history.joinAfter) >= end) return 'Entrou após o evento'
  const first = Date.parse(history.firstSeen)
  if (first > end || first > clashDate(event?.startTime)) return 'Entrada recente · conferir elegibilidade'
  return ''
}
