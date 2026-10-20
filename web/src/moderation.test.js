import test from 'node:test'
import assert from 'node:assert/strict'
import { moderationSummary, moderationLabel, moderationStatus, moderationMessage, punishmentListMessage } from './moderation.js'
const now = Date.now()
const warning = overrides => ({ playerTag: '#P0Y8LQ', playerName: 'Jogador', kind: 'warning', status: 'active', sector: 'wars', createdAt: new Date(now).toISOString(), expiresAt: new Date(now + 30*86400000).toISOString(), reason: 'Zero ataques na guerra.', ...overrides })
test('Primeiro aviso acompanha cooperação; segundo solicita decisão sem aplicar punição', () => {
  const [first] = moderationSummary([warning()])
  assert.equal(first.warnings, 1); assert.equal(first.needsDecision, false)
  assert.match(moderationLabel(first), /1º aviso/)
  const [second] = moderationSummary([warning(), warning({ source: 'cwl' })])
  assert.equal(second.needsDecision, true); assert.equal(second.punishments, 0)
  assert.match(moderationLabel(second), /decidir punição/)
})
test('Raids a conferir, avisos removidos ou justificados não causam punições', () => {
  const [player] = moderationSummary([warning(), warning({ status: 'pending' }), warning({ status: 'dismissed' }), warning({ status: 'justified' }), warning({ status: 'resolved' })])
  assert.equal(player.warnings, 1); assert.equal(player.pending, 1); assert.equal(player.needsDecision, false)
  assert.deepEqual(moderationSummary([warning({ status: 'dismissed' })]), [])
})
test('Punições manuais e mensagens preservam a decisão da liderança', () => {
  const record = warning({ kind: 'punishment', action: 'Fora das próximas duas guerras.' })
  const [player] = moderationSummary([warning(), warning(), record])
  assert.equal(player.punishments, 1); assert.equal(player.needsDecision, false)
  assert.match(moderationLabel(player), /Punição registrada/)
  assert.match(moderationMessage(record, 'Império'), /Medida definida: Fora das próximas duas guerras/)
  assert.match(moderationMessage(warning({ status: 'pending' }), 'Império'), /em conferência/)
})

test('WhatsApp punishment list includes only active decisions and omits internal notes',()=>{
  const punishment=warning({kind:'punishment',action:'Fora de duas guerras.',notes:'Observação privada.'})
  const records=[punishment,...['pending','dismissed','justified','resolved'].map(status=>({...punishment,status,playerName:'Fora da lista '+status})),warning(),{...punishment,action:'',playerName:'Sem medida'}]
  const text=punishmentListMessage(records,'Meu clã')
  assert.match(text,/Meu clã · Punições em vigor/)
  assert.match(text,/Jogador \(#P0Y8LQ\)/)
  assert.match(text,/Motivo: Zero ataques/)
  assert.match(text,/Medida definida: Fora de duas guerras/)
  assert.ok(!text.includes('Observação privada'))
  assert.ok(!text.includes('Fora da lista'))
  assert.ok(!text.includes('Sem medida'))
  assert.equal(punishmentListMessage(records.filter(r=>r.status!=='active'),'Meu clã'),'')
})

test('Individual punishment selection keeps different accounts separate and marks unresolved historical records accurately',()=>{
  const records=[warning({kind:'punishment',action:'Medida 1'}),warning({kind:'punishment',action:'Medida 2',playerTag:'#QY8L0P',playerName:'Outra conta'})]
  const text=punishmentListMessage(records,'Meu clã','P0Y8LQ')
  assert.match(text,/Medida 1/)
  assert.ok(!text.includes('Outra conta'))
  assert.equal(punishmentListMessage(records,'Meu clã','#PPPP'),'')
  const pending=moderationMessage({...records[0],status:'pending'},'Meu clã')
  assert.match(pending,/Situação: A conferir/)
  assert.match(pending,/Medida em avaliação/)
  assert.ok(!pending.includes('Medida definida'))
  const removed=moderationMessage(warning({status:'dismissed'}),'Meu clã')
  assert.match(removed,/Situação: Removido/)
  assert.ok(!removed.includes('Uma segunda ocorrência'))
})

test('Degrees add different sectors but expulsion requires one sector with four or all four sectors', () => {
  for (const [sectors, degree] of [
    [['wars', 'capital'], 2],
    [['wars', 'capital', 'games'], 3],
    [['wars', 'wars', 'capital', 'capital'], 3],
    [['wars', 'wars', 'capital', 'games'], 3],
    [['wars', 'wars', 'wars', 'wars'], 4],
    [['wars', 'capital', 'games', 'replays'], 4],
  ]) {
    const [player] = moderationSummary(sectors.map(sector => warning({ sector })), now)
    assert.equal(player.degree, degree)
    assert.equal(player.needsDecision, true)
    assert.equal(player.warnings, sectors.length)
  }
})

test('An existing lesser punishment cannot hide a newly reached expulsion degree', () => {
  const records = ['wars', 'capital', 'games', 'replays'].map(sector => warning({ sector }))
  records.push(warning({ kind: 'punishment', sanction: 'two_wars', action: 'Fora de duas guerras.' }))
  let [player] = moderationSummary(records, now)
  assert.equal(player.needsDecision, true)
  assert.match(moderationLabel(player), /Grau 4.*registrar expulsão/)
  records.push(warning({ kind: 'punishment', sanction: 'expulsion', action: 'Expulsão do clã.' }))
  ;[player] = moderationSummary(records, now)
  assert.equal(player.needsDecision, false)
  assert.match(moderationLabel(player), /Expulsão registrada/)
})

test('Expired warnings disappear from degrees at the boundary but remain in messages as history', () => {
  const expired = warning({ expiresAt: new Date(now).toISOString() })
  assert.equal(moderationStatus(expired, now - 1), 'active')
  assert.equal(moderationStatus(expired, now), 'expired')
  assert.deepEqual(moderationSummary([expired], now), [])
  const message = moderationMessage(expired, 'Meu clã')
  assert.match(message, /Situação: Expirado/)
  assert.match(message, /Não conta para punições/)
  assert.ok(!message.includes('2º aviso:'))
  const [player] = moderationSummary([expired, warning({ sector: 'games' })], now)
  assert.equal(player.degree, 1)
})

test('Legacy unclassified warnings need review and previous-cycle punishments do not cover new warnings', () => {
  const [legacy] = moderationSummary([warning({ sector: '', source: 'manual' })], now)
  assert.equal(legacy.warnings, 0)
  assert.equal(legacy.pending, 1)
  const [player] = moderationSummary([warning(), warning({ sector: 'capital' }), warning({ kind: 'punishment', sanction: 'two_wars', createdAt: new Date(now - 40*86400000).toISOString() })], now)
  assert.equal(player.needsDecision, true)
})

test('A punishment activated after pending review covers the current warning cycle', () => {
  const decision = warning({ kind: 'punishment', sanction: 'two_wars', createdAt: new Date(now - 40*86400000).toISOString(), appliedAt: new Date(now).toISOString() })
  const [player] = moderationSummary([warning(), warning({ sector: 'capital' }), decision], now)
  assert.equal(player.needsDecision, false)
})
