import test from 'node:test'
import assert from 'node:assert/strict'
import { moderationSummary, moderationLabel, moderationMessage, punishmentListMessage } from './moderation.js'
const warning = overrides => ({ playerTag: '#P0Y8LQ', playerName: 'Jogador', kind: 'warning', status: 'active', reason: 'Zero ataques na guerra.', ...overrides })
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
