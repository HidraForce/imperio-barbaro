import test from 'node:test'
import assert from 'node:assert/strict'
import { membersCSV, moderationCSV, membershipLabel } from './admin-reports.js'
import { buildAttackOverview } from './attack-overview.js'
import { battlePending, raidPending } from './battle-report.js'

test('Members CSV exports roster values, observed entry and safe spreadsheet text', () => {
  const member = {name:'=1+1',tag:'#PYQ',role:'coLeader',expLevel:200,trophies:5000,donations:10,membership:{newMember:true,detectedJoin:true,firstSeen:'2026-10-13T10:00:00Z',joinAfter:'2026-10-12T10:00:00Z'}}
  const output = membersCSV([member], 'Clã "Bárbaro"')
  assert.ok(output.startsWith('\uFEFF'))
  assert.ok(output.includes('"Clã ""Bárbaro""","\'=1+1","#PYQ","Colíder","200","5000","10"'))
  assert.ok(output.includes('Novo · entrada/retorno observado nos últimos 7 dias'))
  assert.ok(output.includes(member.membership.firstSeen))
  assert.match(membershipLabel({membership:{firstSeen:'2026-10-13'}}), /entrada desconhecida/)
})

test('Discipline CSV preserves status, decisions, event dates, multiline reasons and notes', () => {
  const output = moderationCSV([{playerName:'+SUM(1)',playerTag:'#PYQ',kind:'punishment',status:'pending',reason:'Faltou, conferir\n"guerra"',action:'Fora de 1 guerra',notes:'Justificativa pendente',source:'cwl',eventStart:'20261010T070000.000Z',eventEnd:'20261011T070000.000Z',createdAt:'2026-10-11T08:00:00Z'}], 'Clã')
  assert.ok(output.includes('"\'+SUM(1)"'))
  assert.ok(output.includes('"Punição","A conferir","Faltou, conferir\n""guerra""","Fora de 1 guerra","Justificativa pendente","CWL"'))
  assert.ok(output.includes('"20261010T070000.000Z","20261011T070000.000Z"'))
})

test('Discipline CSV exports sector and expiration without counting expired warnings as active', () => {
  const output = moderationCSV([{ playerName: 'Jogador', playerTag: '#PYQ', kind: 'warning', status: 'active', sector: 'games', expiresAt: '2020-01-30T12:00:00Z' }], 'Clã')
  assert.match(output, /Expirado/)
  assert.match(output, /Jogos/)
  assert.match(output, /2020-01-30T12:00:00Z/)
})

test('Recent arrivals are neutral in overview and excluded from accusatory messages', () => {
  const member = {tag:'#PYQ',name:'Novo',membership:{detectedJoin:true,newMember:true,joinAfter:'2026-10-12T07:00:00Z',firstSeen:'2026-10-13T07:00:00Z'}}
  const event = {state:'warEnded',startTime:'20261010T070000.000Z',endTime:'20261012T070000.000Z',clan:{tag:'#CLAN',members:[member]}}
  const raid = {rosterDuringRaid:true,roster:[member],season:{...event,members:[]}}
  const row = buildAttackOverview({members:[member],war:event,leagueWar:event,leagueMembers:[member],raidRecord:raid})[0]
  assert.equal(row.pendingCount,0)
  assert.equal(row.statuses.raids.label,'Entrou após o evento')
  assert.deepEqual(battlePending({war:event},'CLAN','Guerra',2,[member]),[])
  assert.deepEqual(raidPending(raid,Date.now(),[member]),[])
  const uncertain = {...member,membership:{...member.membership,joinAfter:'2026-10-09T07:00:00Z'}}
  assert.match(buildAttackOverview({members:[uncertain],war:event})[0].statuses.war.label,/conferir elegibilidade/)
})

test('A roster captured after a raid cannot invent attack debts', () => {
  const member = {tag:'#PYQ',name:'Novo'}
  const record = {rosterDuringRaid:false,roster:[member],season:{state:'ended',members:[]}}
  assert.deepEqual(raidPending(record),[])
  const row = buildAttackOverview({members:[member],raidRecord:record})[0]
  assert.equal(row.pendingCount,0)
  assert.equal(row.statuses.raids.label,'Lista histórica não confirmada')
})
