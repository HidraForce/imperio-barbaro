import { moderationStatus, moderationSector, moderationSectors } from './moderation.js'
import { csvCell } from './report-csv.js'

export const moderationStatuses = { pending: 'A conferir', active: 'Ativo', justified: 'Justificado', resolved: 'Encerrado', dismissed: 'Removido', expired: 'Expirado' }
export function roleLabel(role) { return ({leader:'Líder',coLeader:'Colíder',admin:'Ancião',member:'Membro'})[role] || 'Membro' }
const csv = rows => '\uFEFF' + rows.map(row => row.map(csvCell).join(',')).join('\r\n')

export function membershipLabel(member) {
  const history = member.membership
  if (!history?.firstSeen) return 'Histórico indisponível'
  if (!history.detectedJoin) return 'Já estava no primeiro registro; entrada desconhecida'
  return history.newMember ? 'Novo · entrada/retorno observado nos últimos 7 dias' : 'Entrada/retorno observado'
}

export function membersCSV(members, clanName) {
  return csv([
    ['Clã','Jogador','Tag','Cargo','Nível','Troféus','Doações','Doações recebidas','Estrelas de guerra','Entrada no clã','Primeira observação desta permanência (UTC)','Última ausência confirmada (UTC)'],
    ...members.map(m => [clanName,m.name,m.tag,roleLabel(m.role),m.expLevel,m.trophies,m.donations,m.donationsReceived,m.warStars,membershipLabel(m),m.membership?.firstSeen,m.membership?.joinAfter]),
  ])
}

export function moderationCSV(records, clanName) {
  return csv([
    ['Clã','Jogador','Tag','Lista','Situação','Motivo','Medida','Observações','Origem','Início do evento (UTC)','Fim do evento (UTC)','Criado em (UTC)','Atualizado em (UTC)','Setor','Expira em (UTC)','Aviso confirmado em (UTC)','Punição aplicada em (UTC)'],
    ...records.map(r => [clanName,r.playerName,r.playerTag,r.kind === 'punishment' ? 'Punição' : 'Aviso',moderationStatuses[moderationStatus(r)] || r.status,r.reason,r.action,r.notes,({manual:'Manual',war:'Guerra',cwl:'CWL',raids:'Raids'})[r.source] || r.source,r.eventStart,r.eventEnd,r.createdAt,r.updatedAt,moderationSectors[moderationSector(r)] || 'A definir',r.expiresAt,r.issuedAt,r.appliedAt]),
  ])
}

export function downloadCSV(content, filename) {
  const url = URL.createObjectURL(new Blob([content], {type:'text/csv;charset=utf-8'}))
  const link = document.createElement('a')
  link.href = url; link.download = filename; link.click()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}
