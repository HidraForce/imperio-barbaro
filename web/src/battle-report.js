import { clashTime, confirmedEnd, completeWarAttacks, completeRaidAttacks } from './final-activity.js'
import { csvCell } from './report-csv.js'
import { entryEligibility } from './member-eligibility.js'
import { playerMessageLabel } from './contact-numbers.js'

const tagKey = tag => String(tag || '').replace(/^#/, '').toUpperCase()
export function ownSide(war, clanTag) {
  return [war?.clan, war?.opponent].find(side => side?.tag && tagKey(side.tag) === tagKey(clanTag)) || null
}
export function battleStatus(war, member, fallback = 1) {
  const used = member?.attacks?.length || 0
  const limit = war?.attacksPerMember || fallback
  if (!member) return { label: 'Não escalado', tone: 'neutral', remaining: 0 }
  if (!['inWar', 'warEnded'].includes(war?.state)) return { label: war?.state === 'preparation' ? 'Preparação' : 'Dados indisponíveis', tone: 'neutral', remaining: 0, used, limit }
  const remaining = Math.max(0, limit - used)
  return { label: remaining ? (war.state === 'warEnded' ? 'Não realizados' : 'Pendentes') : 'Concluídos', tone: remaining ? 'pending' : 'complete', remaining, used, limit }
}
export function seasonRows(group, rounds, clanTag) {
  const people = new Map()
  const add = member => { if (member?.tag) people.set(tagKey(member.tag), { ...people.get(tagKey(member.tag)), ...member }) }
  const registered = (group?.clans || []).find(c => tagKey(c.tag) === tagKey(clanTag))
  for (const member of registered?.members || []) add(member)
  for (const entry of rounds) for (const member of ownSide(entry.war, clanTag)?.members || []) add(member)
  return [...people.values()].map(member => ({ ...member, rounds: rounds.map(entry => {
    const participant = ownSide(entry.war, clanTag)?.members?.find(m => tagKey(m.tag) === tagKey(member.tag))
    if (!entry.war || !ownSide(entry.war, clanTag)) return { label: entry.error ? 'Falha na consulta' : 'Rodada não disponível', tone: 'neutral', remaining: 0 }
    return battleStatus(entry.war, participant)
  }) })).sort((a,b) => a.name.localeCompare(b.name, 'pt-BR'))
}
export function battleCSV({ clanName, clanTag, type, season = '', rounds, registered = [] }) {
  const rows = [['Relatório', type], ['Clã', clanName, '#'+tagKey(clanTag)], ['Temporada', season], [], ['Rodada','Situação da batalha','Jogador','Tag','Escalação','Ataques usados','Ataques permitidos','Pendentes','Situação','Estrelas','Destruição total (%)','Alvos']]
  for (const entry of rounds) {
    const side = ownSide(entry.war, clanTag)
    if (!side) { rows.push([entry.round || '', entry.error || entry.message || 'Dados indisponíveis']); continue }
    const people = new Map(registered.map(m => [tagKey(m.tag), { member:m, selected:false }]))
    for (const member of side.members || []) people.set(tagKey(member.tag), {member,selected:true})
    for (const {member,selected} of people.values()) {
      const status = battleStatus(entry.war, selected ? member : null, type === 'Guerra' ? 2 : 1)
      const attacks = selected ? member.attacks || [] : []
      rows.push([entry.round || '', ({preparation:'Preparação',inWar:'Em andamento',warEnded:'Encerrada'})[entry.war.state] || 'Indisponível',member.name,member.tag,selected?'Escalado':'Não escalado', selected?status.used:'', selected?status.limit:'',status.remaining,status.label,attacks.reduce((sum,a)=>sum+(a.stars||0),0),attacks.reduce((sum,a)=>sum+(a.destructionPercentage||0),0),attacks.map(a=>a.defenderTag).join('; ')])
    }
  }
  return '\uFEFF'+rows.map(row=>row.map(csvCell).join(',')).join('\r\n')
}
export function battlePending(entry, clanTag, label, fallback = 1, members = []) {
  const side=ownSide(entry.war,clanTag), ended=entry.war?.state==='warEnded'
  const limit=entry.war?.attacksPerMember??(fallback===1?1:null)
  if(ended&&(limit!==fallback||!confirmedEnd(entry.war,'warEnded')||!completeWarAttacks(entry.war,side,limit)))return []
  if(!ended&&Number.isFinite(clashTime(entry.war?.endTime))&&clashTime(entry.war.endTime)<=Date.now())return []
  return (side?.members || []).flatMap(member => {
    if (entryEligibility(members.find(m => tagKey(m.tag) === tagKey(member.tag)), entry.war)) return []
    const status = battleStatus(entry.war, member, fallback)
    return status.remaining && (!ended || status.used === 0) ? [{tag:member.tag,name:member.name,activity:label,used:status.used,limit:status.limit,ended:entry.war.state==='warEnded',verify:false}] : []
  })
}
export function filterBattleWarnings(items, attacks = 'all') {
  return items.filter(item => Number.isInteger(item.used) && item.used >= 0
    && Number.isFinite(item.limit) && item.used < item.limit
    && (attacks === 'all' || item.used === Number(attacks)))
}
export function raidPending(record, now = Date.now(), members = []) {
  if (!record?.season) return []
  const season = record.season
  const ended=confirmedEnd(season,'ended',now)
  if(season.state==='ended'&&(!ended||!completeRaidAttacks(season)))return []
  if(season.state!=='ended'&&(season.state!=='ongoing'||clashTime(season.endTime)<=now))return []
  const players = new Map((season.members || []).map(m=>[tagKey(m.tag),m]))
  const roster = new Map((record.roster || []).map(m=>[tagKey(m.tag),m]))
  const combined = new Map([...roster,...players])
  return [...combined.values()].flatMap(member=>{
    if (entryEligibility(members.find(m => tagKey(m.tag) === tagKey(member.tag)), season)) return []
    const attacker = players.get(tagKey(member.tag)), used = attacker ? attacker.attacks : 0
    if(!Number.isInteger(used)||used<0)return []
    if (!attacker && !record.rosterDuringRaid) return []
    const bonus=attacker?.bonusAttackLimit??0
    const limit=attacker?.attackLimit==null?null:attacker.attackLimit+bonus
    if(attacker&&(!Number.isInteger(attacker.attackLimit)||attacker.attackLimit<1||!Number.isInteger(bonus)||bonus<0||bonus>1||limit>6))return []
    if (limit == null ? used > 0 : used >= limit) return []
    return [{tag:member.tag,name:member.name,activity:'Raids · '+String(season.startTime||'').slice(0,8),used,limit,ended,verify:!attacker||!record.rosterDuringRaid}]
  })
}
export function pendingMessage({clanName,clanTag,items,playerTag='',phones={}}) {
  const chosen = items.filter(item=>!playerTag||tagKey(item.tag)===tagKey(playerTag))
  if (!chosen.length) return ''
  const closed = chosen.filter(item=>item.ended&&item.current!==false&&item.current!==null), active = chosen.filter(item=>!item.ended&&item.current!==false&&item.current!==null), former=chosen.filter(item=>item.current===false), unconfirmed=chosen.filter(item=>item.current===null)
  const lines = [`*${clanName || 'Clã'} · #${tagKey(clanTag)}*`, playerTag ? `*Atenção, ${playerMessageLabel(chosen[0].name,chosen[0].tag,phones,chosen[0].current)}*` : '*Aviso de pendências de ataques*', '']
  const line = item => `• ${playerMessageLabel(item.name,item.tag,phones,item.current)} — ${item.activity}: ${item.used}${item.limit==null?' ataques registrados':'/'+item.limit+' ataques usados'}.`
  if (closed.length) lines.push('*Atividades encerradas — justificativa necessária*',...closed.map(line),'','Quem consta acima deve apresentar uma justificativa à liderança pela inadimplência nos ataques. Sem justificativa, ficará sob aviso.','')
  if (active.length) lines.push('*Atividades em andamento*',...active.map(line),'','O prazo ainda está aberto. Realize os ataques restantes antes do encerramento; se houver algum impedimento, avise a liderança.','')
  if (former.length) lines.push('*Fora do clã atual — histórico de ataques*',...former.map(line),'','Esses jogadores não constam na lista atual deste clã. Os registros acima são históricos; confira as datas e o vínculo com a liderança. Esta seção não é uma cobrança aos membros atuais.','')
  if (unconfirmed.length) lines.push('*Vínculo não conferido — confirmar lista atual*',...unconfirmed.map(line),'','Não foi possível conferir a lista atual deste clã. Confirme o vínculo e as datas dos registros com a liderança antes de fazer qualquer cobrança.','')
  if (chosen.some(item=>item.verify)) lines.push('Nas raids, confirme a elegibilidade e eventual participação em outro clã com a liderança antes de aplicar o aviso.','')
  lines.push('Caso já tenha se justificado ou identifique uma divergência nos registros, informe a liderança para conferência.')
  return lines.join('\n')
}
