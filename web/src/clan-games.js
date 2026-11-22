import { csvCell } from './report-csv.js'
import { roleLabel } from './admin-reports.js'

const statusLabels = {
  completed: 'Meta cumprida',
  contributing: 'Contribuindo',
  zero: 'Zero registrado',
  review: 'A conferir',
  ineligible: 'Não elegível',
  upcoming: 'Aguardando início',
}
const statusOrder = { zero: 0, contributing: 1, review: 2, upcoming: 3, completed: 4, ineligible: 5 }

export function clanGamesStatus(row) {
  return statusLabels[row.status] || 'A conferir'
}

export function clanGamesPoints(row) {
  if (!Number.isFinite(row.points) || row.points < 0) return '—'
  const points = row.points.toLocaleString('pt-BR')
  return row.partial ? `${points} observados` : points
}

export function buildObservedClanGamesProgress(items, event = {}) {
  const accounts = items.filter(row => row.current === true && row.status !== 'ineligible')
  const observed = accounts.filter(row => Number.isFinite(row.points) && row.points >= 0)
  const baselineTimes = accounts.map(row => ({ value: row.baselineAt, time: Date.parse(row.baselineAt) })).filter(baseline => Number.isFinite(baseline.time)).sort((left, right) => left.time - right.time)
  const firstBaseline = baselineTimes[0]
  const allPartial = accounts.length > 0 && accounts.every(row => row.partial === true)
  return {
    points: observed.length ? observed.reduce((points, row) => points + row.points, 0) : null,
    participants: observed.filter(row => row.points > 0).length,
    zero: observed.filter(row => row.points === 0).length,
    missing: accounts.length - observed.length,
    accounts: accounts.length,
    firstBaselineAt: firstBaseline?.value || '',
    allPartial,
    lateBaseline: allPartial && Number.isFinite(Date.parse(event.startTime)) && Boolean(firstBaseline) && firstBaseline.time >= Date.parse(event.startTime),
  }
}

export function visibleClanGames(items, status = 'all', presence = 'all') {
  return items.filter(row => {
    if (presence === 'current' && row.current !== true) return false
    if (presence === 'former' && row.current !== false) return false
    if (status === 'observed-positive') return Number.isFinite(row.points) && row.points > 0
    if (status === 'observed-zero') return row.points === 0
    if (status === 'below') return !row.partial && ['zero', 'contributing'].includes(row.status) && Number.isFinite(row.points) && row.points < row.target
    return status === 'all' || row.status === status
  }).sort((left, right) => {
    const statusDifference = (statusOrder[left.status] ?? 2) - (statusOrder[right.status] ?? 2)
    if (statusDifference) return statusDifference
    const leftPoints = Number.isFinite(left.points) ? left.points : Infinity
    const rightPoints = Number.isFinite(right.points) ? right.points : Infinity
    return leftPoints - rightPoints || String(left.name || left.tag).localeCompare(String(right.name || right.tag), 'pt-BR')
  })
}

export function clanGamesCSV(items, event, clanId) {
  const rows = [
    ['Clã', 'Edição', 'Jogador', 'Tag', 'Cargo', 'Centro de vila', 'Vínculo na coleta', 'Pontos observados', 'Precisão', 'Meta', 'Situação', 'Conferência', 'Referência inicial (UTC)', 'Última consulta do jogador (UTC)'],
    ...items.map(row => [
      clanId, event.season, row.name, row.tag, roleLabel(row.role), row.townHallLevel,
      row.current === true ? 'No clã na última coleta' : row.current === false ? 'Fora do clã na última coleta' : 'Vínculo não confirmado',
      Number.isFinite(row.points) && row.points >= 0 ? row.points : '',
      row.partial ? 'Parcial / a conferir' : Number.isFinite(row.points) && row.points >= 0 ? 'Diferença desde a referência' : 'Indisponível',
      row.target, clanGamesStatus(row), row.reason, row.baselineAt, row.updatedAt,
    ]),
  ]
  return '\uFEFF' + rows.map(row => row.map(csvCell).join(',')).join('\r\n')
}
