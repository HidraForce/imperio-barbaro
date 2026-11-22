import test from 'node:test'
import assert from 'node:assert/strict'
import { clanGamesCSV, clanGamesPoints, clanGamesStatus, visibleClanGames } from './clan-games.js'

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
