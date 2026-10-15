import test from 'node:test'
import assert from 'node:assert/strict'
import { buildAttackOverview } from './attack-overview.js'

const members = [{ tag: '#A', name: 'Ana' }, { tag: '#B', name: 'Bruno' }, { tag: '#C', name: 'Carlos' }]
const raidRecord = { roster: members.slice(0, 2), season: { state: 'ongoing', members: [{ ...members[0], attacks: 5, attackLimit: 5, bonusAttackLimit: 1 }] } }
const war = { state: 'inWar', attacksPerMember: 2, clan: { members: [{ ...members[0], attacks: [{}] }, { ...members[1], attacks: [{}, {}] }] } }

test('Ataques parciais e bônus pendentes ficam vermelhos; todos os ataques usados ficam verdes', () => {
  const rows = buildAttackOverview({ members, raidRecord, war })
  const ana = rows.find(r => r.tag === '#A')
  const bruno = rows.find(r => r.tag === '#B')
  assert.equal(ana.statuses.raids.tone, 'missing')
  assert.match(ana.statuses.raids.label, /5\/6/)
  assert.equal(ana.statuses.war.tone, 'missing')
  assert.match(ana.statuses.war.label, /1\/2/)
  assert.equal(bruno.statuses.war.tone, 'complete')
  assert.equal(bruno.statuses.raids.tone, 'missing')
  assert.equal(rows[0].tag, '#A')
})

test('Jogadores fora da escalação e da lista de raids não recebem pendências', () => {
  const carlos = buildAttackOverview({ members, raidRecord, war }).find(r => r.tag === '#C')
  assert.equal(carlos.pendingCount, 0)
  assert.equal(carlos.statuses.war.label, 'Não escalado')
  assert.equal(carlos.statuses.raids.label, 'Fora da lista registrada')
})

test('Preparação, ausência de guerra, erro e carregamento ficam neutros', () => {
  for (const value of [{ war: { ...war, state: 'preparation' } }, { war: { state: 'notInWar' } }, { war, warError: 'Falha na API' }, { war, loading: true }, {}]) {
    const rows = buildAttackOverview({ members, ...value })
    assert.ok(rows.every(r => r.statuses.war.tone === 'neutral' && !r.statuses.war.pending))
  }
})

test('CWL considera apenas a escalação selecionada e um ataque por jogador', () => {
  const rows = buildAttackOverview({ members, leagueWar: { state: 'inWar' }, leagueMembers: [{ ...members[0], attacks: [] }, { ...members[1], attacks: [{}] }] })
  assert.equal(rows.find(r => r.tag === '#A').statuses.cwl.tone, 'missing')
  assert.equal(rows.find(r => r.tag === '#B').statuses.cwl.tone, 'complete')
  assert.equal(rows.find(r => r.tag === '#C').statuses.cwl.tone, 'neutral')
})

test('Ataques não realizados após o encerramento continuam visíveis, inclusive de ex-membros', () => {
  const rows = buildAttackOverview({ members: [], war: { ...war, state: 'warEnded' } })
  assert.equal(rows.length, 2)
  assert.ok(rows.every(r => !r.current))
  assert.match(rows.find(r => r.tag === '#A').statuses.war.label, /Não realizados/)
})

test('Não presume conclusão quando o limite de ataques da raid é desconhecido', () => {
  const rows = buildAttackOverview({ members, raidRecord: { roster: members, season: { members: [{ ...members[0], attacks: 3 }] } } })
  assert.equal(rows.find(r => r.tag === '#A').statuses.raids.tone, 'neutral')
})
