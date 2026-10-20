import { playerMessageLabel } from './contact-numbers.js'
import { clanPresence } from './member-presence.js'

const tagKey = value => String(value || '').replace(/^#/, '').toUpperCase()
export const moderationSectors = { games: 'Jogos', capital: 'Capital', wars: 'Guerras', replays: 'Replays' }
export const moderationSanctions = { two_wars: 'Fora das próximas duas guerras.', demotion: 'Perda de cargo.', expulsion: 'Expulsão do clã.' }
export function moderationSector(record) { return record.sector || ({ war: 'wars', cwl: 'wars', raids: 'capital' })[record.source] || '' }
export function moderationStatus(record, now = Date.now()) {
  if (record.kind !== 'warning' || record.status !== 'active') return record.status
  const expires = Date.parse(record.expiresAt)
  if (!moderationSectors[moderationSector(record)] || !Number.isFinite(expires)) return 'pending'
  return expires <= now ? 'expired' : 'active'
}
export function moderationSummary(records = [], now = Date.now()) {
  const players = new Map()
  for (const record of records) {
    const status = moderationStatus(record, now)
    if (!['active', 'pending'].includes(status)) continue
    const tag = tagKey(record.playerTag)
    const player = players.get(tag) || { tag: record.playerTag, name: record.playerName, warnings: 0, punishments: 0, pending: 0, sectors: Object.fromEntries(Object.keys(moderationSectors).map(key => [key, 0])), cycleStart: Infinity, expiresAt: '', decisions: [] }
    if (status === 'pending') player.pending++
    else if (record.kind === 'warning') {
      player.warnings++
      player.sectors[moderationSector(record)]++
      player.cycleStart = Math.min(player.cycleStart, Date.parse(record.issuedAt || record.createdAt))
      player.expiresAt = record.expiresAt
    } else if (record.kind === 'punishment') {
      player.punishments++
      player.decisions.push(record)
    }
    players.set(tag, player)
  }
  return [...players.values()].map(player => {
    const counts = Object.values(player.sectors)
    const degree = counts.some(count => count >= 4) || counts.every(count => count >= 1) ? 4 : Math.min(player.warnings, 3)
    const covered = player.decisions.some(record => Date.parse(record.appliedAt || record.createdAt) >= player.cycleStart && (degree < 4 || record.sanction === 'expulsion'))
    return { ...player, degree, needsDecision: degree >= 2 && !covered }
  }).sort((a, b) => Number(b.needsDecision) - Number(a.needsDecision) || b.degree - a.degree || b.punishments - a.punishments || b.warnings - a.warnings || a.name.localeCompare(b.name, 'pt-BR'))
}
export function moderationLabel(player) {
  if (!player) return 'Sem avisos'
  if (player.degree === 4) return `Grau 4 · ${player.needsDecision ? 'registrar expulsão' : 'Expulsão registrada'}`
  if (player.degree === 3) return `Grau 3 · reincidência · ${player.needsDecision ? 'decidir punição' : 'Punição registrada'}`
  if (player.degree === 2) return `Grau 2 · 2º aviso · ${player.needsDecision ? 'decidir punição' : 'Punição registrada'}`
  if (player.warnings) return 'Grau 1 · 1º aviso · advertência'
  if (player.punishments) return 'Punição registrada'
  return `${player.pending} ocorrência(s) · conferir antes de contar como aviso`
}
export function moderationSectorCounts(player) {
  return Object.entries(moderationSectors).map(([key, label]) => `${label}: ${player.sectors[key]}`).join(' · ')
}
export function moderationMessage(record, clanName, phones={}, current=null) {
  const effectiveStatus = moderationStatus(record)
  const status=({pending:'A conferir',active:'Ativo',justified:'Justificado',resolved:'Encerrado',dismissed:'Removido',expired:'Expirado'})[effectiveStatus]||effectiveStatus
  const lines = [`*${clanName}*`, `*${record.kind === 'punishment' ? 'Punição' : 'Aviso'} · ${playerMessageLabel(record.playerName,record.playerTag,phones,current)}*`, 'Situação: '+status, record.reason]
  if (record.kind === 'warning') lines.push('Setor: ' + (moderationSectors[moderationSector(record)] || 'A definir'))
  if (record.expiresAt) lines.push('Validade: ' + new Date(record.expiresAt).toLocaleString('pt-BR'))
  if (current===false) lines.push('Fora do clã atual. Registro histórico preservado para conferência da liderança.')
  if (record.action) lines.push((record.status==='pending'?'Medida em avaliação: ':record.status==='active'?'Medida definida: ':'Medida registrada: ') + record.action)
  if (effectiveStatus === 'pending') lines.push('Registro em conferência pela liderança; apresente sua justificativa antes de qualquer decisão.')
  else if (current!==false && effectiveStatus==='active' && record.kind === 'warning') lines.push('1º aviso: advertência antes de qualquer punição. 2º aviso: fora de duas guerras ou perda de cargo. Quatro avisos no mesmo setor ou pelo menos um em cada um dos quatro setores (Jogos, Capital, Guerras e Replays): expulsão. Os avisos expiram após 30 dias sem reincidência; um novo aviso reinicia o prazo de todos os avisos ainda ativos. Apresente sua justificativa se houver um impedimento.')
  if (effectiveStatus === 'expired') lines.push('Aviso expirado após 30 dias sem reincidência. Não conta para punições.')
  return lines.join('\n\n')
}

export function punishmentListMessage(records, clanName, playerTag='', phones={}, members) {
  const active=records.filter(record=>record.kind==='punishment'&&record.status==='active'&&record.action?.trim()&&(!playerTag||tagKey(record.playerTag)===tagKey(playerTag)))
  if(!active.length)return ''
  const lines=[`*${clanName} · Punições em vigor*`,'Medidas registradas e definidas pela liderança:','']
  active.forEach((record,index)=>lines.push(`${index+1}. *${playerMessageLabel(record.playerName,record.playerTag,phones,clanPresence(record.playerTag,members))}*`,'Motivo: '+record.reason,'Medida definida: '+record.action,''))
  lines.push('Se houver uma divergência ou justificativa ainda não considerada, procure a liderança para conferência.')
  return lines.join('\n')
}
