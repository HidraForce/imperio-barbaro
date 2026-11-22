import test from 'node:test'
import assert from 'node:assert/strict'
import { buildObservedClanGamesProgress, clanGamesCSV, clanGamesPoints, clanGamesStatus, visibleClanGames } from './clan-games.js'

test('Dados ausentes e pontuações parciais não aparecem como zero confirmado', () => {
  assert.equal(clanGamesPoints({ points: null }), '—')
  assert.equal(clanGamesPoints({}), '—')
  assert.equal(clanGamesPoints({ points: -1 }), '—')
  assert.equal(clanGamesPoints({ points: 0, partial: true }), '0 observados')
  assert.equal(clanGamesPoints({ points: 1250, partial: true }), '1.250 observados')
  assert.equal(clanGamesPoints({ points: 0 }), '0')
  assert.equal(clanGamesStatus({ status: 'review', points: 4000 }), 'A conferir')
  assert.equal(clanGamesStatus({ status: 'new-status', points: 0 }), 'A conferir')
})

test('Filtros preservam incertezas e distinguem membros atuais dos antigos', () => {
  const items = [
    { tag: '#OLD', name: 'Antigo', current: false, status: 'zero', points: 0, target: 4000 },
    { tag: '#ZERO', name: 'Zero', current: true, status: 'zero', points: 0, target: 4000 },
    { tag: '#PART', name: 'Parcial', current: true, status: 'review', points: 1000, target: 4000, partial: true },
    { tag: '#DONE', name: 'Concluído', current: true, status: 'completed', points: 4000, target: 4000 },
    { tag: '#NULL', name: 'Ausente', current: true, status: 'review', points: null, target: 4000 },
    { tag: '#WORK', name: 'Contribuindo', current: true, status: 'contributing', points: 1500, target: 4000 },
  ]
  assert.deepEqual(visibleClanGames(items, 'below', 'current').map(row => row.tag), ['#ZERO', '#WORK'])
  assert.deepEqual(visibleClanGames(items, 'zero', 'former').map(row => row.tag), ['#OLD'])
  assert.deepEqual(visibleClanGames(items, 'review').map(row => row.tag), ['#PART', '#NULL'])
  assert.equal(items[0].tag, '#OLD')
})

test('CSV mantém falta de dados, precisão parcial e nomes seguros para planilhas', () => {
  const csv = clanGamesCSV([
    { tag: '#P0Y8LQ', name: '=HYPERLINK("https://exemplo.test")', role: 'member', points: null, target: 4000, current: true, status: 'review', reason: '+Conferir', updatedAt: '2026-11-23T08:00:00Z' },
    { tag: '#QY8L0P', name: 'João', role: 'admin', points: 500, target: 4000, current: false, status: 'review', partial: true },
  ], { season: '2026-11' }, '1')
  assert.ok(csv.startsWith('\uFEFF'))
  assert.ok(csv.includes('"\'=HYPERLINK(""https://exemplo.test"")"'))
  assert.ok(csv.includes('"No clã na última coleta","","Indisponível"'))
  assert.ok(csv.includes('"Fora do clã na última coleta","500","Parcial / a conferir"'))
  assert.ok(csv.includes('"\'+Conferir"'))
  assert.ok(csv.includes('"2026-11-23T08:00:00Z"'))
})

test('Aumento observado inclui contas parciais sem atribuir sua pontuação ao total confirmado', () => {
  const items = [
    { current: true, status: 'review', partial: true, points: 400, baselineAt: '2026-11-22T16:07:00Z' },
    { current: true, status: 'review', partial: true, points: 850, baselineAt: '2026-11-22T16:08:00Z' },
    { current: true, status: 'review', partial: true, points: 0 },
    { current: true, status: 'review', partial: true, points: null },
    { current: true, status: 'review', partial: true, points: -1 },
    { current: true, status: 'review', partial: true, points: '600' },
    { current: true, status: 'contributing', partial: false, points: 1000, baselineAt: '2026-11-21T22:00:00Z' },
    { current: true, status: 'review', partial: true, points: 200, reason: 'Confira a participação em outro clã.' },
    { current: false, status: 'review', partial: true, points: 10000 },
    { current: true, status: 'ineligible', points: 50000 },
    { status: 'review', partial: true, points: 1000 },
  ]
  assert.deepEqual(buildObservedClanGamesProgress(items, { startTime: '2026-11-22T08:00:00Z' }), {
    points: 2450, participants: 4, zero: 1, missing: 3, accounts: 8,
    firstBaselineAt: '2026-11-21T22:00:00Z', allPartial: false, lateBaseline: false,
  })
  assert.equal(items[0].status, 'review')
  assert.equal(items[7].reason, 'Confira a participação em outro clã.')
})

test('Falta de pontuação não vira subtotal zero e zero parcial continua sendo apenas observado', () => {
  const missing = buildObservedClanGamesProgress([
    { current: true, status: 'review', partial: true, points: null },
    { current: true, status: 'review', partial: true },
  ])
  assert.equal(missing.points, null)
  assert.equal(missing.missing, 2)
  assert.equal(missing.zero, 0)
  assert.equal(missing.participants, 0)
  const zero = buildObservedClanGamesProgress([{ current: true, status: 'review', partial: true, points: 0 }])
  assert.equal(zero.points, 0)
  assert.equal(zero.zero, 1)
  assert.equal(zero.participants, 0)
  assert.equal(zero.allPartial, true)
  assert.equal(buildObservedClanGamesProgress([]).allPartial, false)
})

test('Referência tardia usa o horário registrado e não inventa data de entrada do jogador', () => {
  const event = { startTime: '2026-11-22T08:00:00Z' }
  const progress = buildObservedClanGamesProgress([
    { current: true, partial: true, status: 'review', points: 850, baselineAt: '2026-11-22T16:08:00Z' },
    { current: true, partial: true, status: 'review', points: 400, baselineAt: '2026-11-22T16:07:00Z' },
    { current: true, partial: true, status: 'review', points: null, baselineAt: 'invalid' },
    { current: false, partial: false, status: 'zero', points: 0, baselineAt: '2026-11-21T10:00:00Z' },
  ], event)
  assert.equal(progress.firstBaselineAt, '2026-11-22T16:07:00Z')
  assert.equal(progress.allPartial, true)
  assert.equal(progress.lateBaseline, true)
  assert.equal(buildObservedClanGamesProgress([{ current: true, partial: true, baselineAt: event.startTime }], event).lateBaseline, true)
  assert.equal(buildObservedClanGamesProgress([{ current: true, partial: true, baselineAt: '2026-11-21T10:00:00Z' }], event).lateBaseline, false)
})

test('Filtros de aumento observado incluem parciais positivas e separam zero de ausência de dados', () => {
  const items = [
    { tag: '#PART', name: 'Parcial', current: true, status: 'review', partial: true, points: 850, target: 4000 },
    { tag: '#ZERO', name: 'Zero parcial', current: true, status: 'review', partial: true, points: 0, target: 4000 },
    { tag: '#NONE', name: 'Sem consulta', current: true, status: 'review', partial: true, points: null, target: 4000 },
    { tag: '#OLD', name: 'Antigo', current: false, status: 'review', partial: true, points: 200, target: 4000 },
    { tag: '#DONE', name: 'Confirmado', current: true, status: 'completed', partial: false, points: 4000, target: 4000 },
  ]
  assert.deepEqual(visibleClanGames(items, 'observed-positive', 'current').map(row => row.tag), ['#PART', '#DONE'])
  assert.deepEqual(visibleClanGames(items, 'observed-zero').map(row => row.tag), ['#ZERO'])
  assert.deepEqual(visibleClanGames(items, 'zero'), [])
  assert.deepEqual(visibleClanGames(items, 'below'), [])
  assert.deepEqual(visibleClanGames(items, 'observed-positive', 'former').map(row => row.tag), ['#OLD'])
})
