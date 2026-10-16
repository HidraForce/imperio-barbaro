import test from 'node:test'
import assert from 'node:assert/strict'
import { moderationSummary, moderationLabel, moderationMessage } from './moderation.js'
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
