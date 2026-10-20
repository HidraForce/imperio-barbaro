import test from 'node:test'
import assert from 'node:assert/strict'
import { sortMembers } from './member-sort.js'
import { membersCSV } from './admin-reports.js'

const members = [
  { name: 'Membro', tag: '#P', role: 'member', trophies: 6000 },
  { name: 'Colíder B', tag: '#Q', role: 'coLeader', trophies: 5000 },
  { name: 'Líder', tag: '#R', role: 'leader', trophies: 1000 },
  { name: 'Ancião', tag: '#Y', role: 'admin', trophies: 3000 },
  { name: 'Colíder A', tag: '#L', role: 'coLeader', trophies: 5000 },
]
test('Trophies sort numerically in both directions without mutating the roster', () => {
  const before = structuredClone(members)
  assert.deepEqual(sortMembers(members).map(m => m.trophies), [6000, 5000, 5000, 3000, 1000])
  assert.deepEqual(sortMembers(members, 'trophies', 'asc').map(m => m.trophies), [1000, 3000, 5000, 5000, 6000])
  assert.deepEqual(members, before)
})
test('Cargo follows clan hierarchy with trophies, name and tag breaking ties', () => {
  assert.deepEqual(sortMembers(members, 'role').map(m => m.name), ['Líder', 'Colíder A', 'Colíder B', 'Ancião', 'Membro'])
  assert.deepEqual(sortMembers(members, 'role', 'asc').map(m => m.role), ['member', 'admin', 'coLeader', 'coLeader', 'leader'])
  const sorted = sortMembers([{ name: 'A', tag: '#A', role: 'member', trophies: 10 }, { name: 'Z', tag: '#Z', role: 'member', trophies: 20 }], 'role')
  assert.equal(sorted[0].tag, '#Z')
  const csv = membersCSV(sortMembers(members, 'role'), 'Clã')
  assert.ok(csv.indexOf('Líder') < csv.indexOf('Colíder A'))
  assert.deepEqual(sortMembers([]), [])
})
