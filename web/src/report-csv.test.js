import test from 'node:test'
import assert from 'node:assert/strict'
import { csvCell } from './report-csv.js'

test('Neutraliza fórmulas em nomes e textos exportados', () => {
  for (const text of ['=HYPERLINK("https://exemplo.test")', '+1+1', '-1+1', '@SUM(A1)', '  =1+1', '\t1', '\n1']) assert.ok(csvCell(text).startsWith('"\''))
})
test('Preserva valores numéricos e escapa aspas', () => {
  assert.equal(csvCell(12), '"12"')
  assert.equal(csvCell(-12), '"-12"')
  assert.equal(csvCell('João "Barba"'), '"João ""Barba"""')
  assert.equal(csvCell(null), '""')
})
