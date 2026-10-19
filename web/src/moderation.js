import { playerMessageLabel } from './contact-numbers.js'

const tagKey = value => String(value || '').replace(/^#/, '').toUpperCase()
export function moderationSummary(records = []) {
  const players = new Map()
  for (const record of records) {
    if (!['active', 'pending'].includes(record.status)) continue
    const tag = tagKey(record.playerTag)
    const player = players.get(tag) || { tag: record.playerTag, name: record.playerName, warnings: 0, punishments: 0, pending: 0 }
    if (record.status === 'pending') player.pending++
    else if (record.kind === 'warning') player.warnings++
    else if (record.kind === 'punishment') player.punishments++
    players.set(tag, player)
  }
  return [...players.values()].map(player => ({ ...player, needsDecision: player.warnings >= 2 && !player.punishments }))
    .sort((a, b) => Number(b.needsDecision) - Number(a.needsDecision) || b.punishments - a.punishments || b.warnings - a.warnings || a.name.localeCompare(b.name, 'pt-BR'))
}
export function moderationLabel(player) {
  if (!player) return 'Sem avisos'
  if (player.punishments) return 'Punição registrada'
  if (player.needsDecision) return `${player.warnings} avisos · decidir punição`
  if (player.warnings) return '1º aviso · acompanhar cooperação'
  return `${player.pending} ocorrência(s) · conferir antes de contar como aviso`
}
export function moderationMessage(record, clanName, phones={}) {
  const status=({pending:'A conferir',active:'Ativo',justified:'Justificado',resolved:'Encerrado',dismissed:'Removido'})[record.status]||record.status
  const lines = [`*${clanName}*`, `*${record.kind === 'punishment' ? 'Punição' : 'Aviso'} · ${playerMessageLabel(record.playerName,record.playerTag,phones)}*`, 'Situação: '+status, record.reason]
  if (record.action) lines.push((record.status==='pending'?'Medida em avaliação: ':record.status==='active'?'Medida definida: ':'Medida registrada: ') + record.action)
  if (record.status === 'pending') lines.push('Registro em conferência pela liderança; apresente sua justificativa antes de qualquer decisão.')
  else if (record.status==='active' && record.kind === 'warning') lines.push('Este aviso registra a falta de cooperação. Uma segunda ocorrência leva à avaliação de punição pela liderança. Apresente sua justificativa se houver um impedimento.')
  return lines.join('\n\n')
}

export function punishmentListMessage(records, clanName, playerTag='', phones={}) {
  const active=records.filter(record=>record.kind==='punishment'&&record.status==='active'&&record.action?.trim()&&(!playerTag||tagKey(record.playerTag)===tagKey(playerTag)))
  if(!active.length)return ''
  const lines=[`*${clanName} · Punições em vigor*`,'Medidas registradas e definidas pela liderança:','']
  active.forEach((record,index)=>lines.push(`${index+1}. *${playerMessageLabel(record.playerName,record.playerTag,phones)}*`,'Motivo: '+record.reason,'Medida definida: '+record.action,''))
  lines.push('Se houver uma divergência ou justificativa ainda não considerada, procure a liderança para conferência.')
  return lines.join('\n')
}
