import test from 'node:test'
import assert from 'node:assert/strict'
import { battleStatus, seasonRows, battleCSV, battlePending, filterBattleWarnings, raidPending, pendingMessage } from './battle-report.js'
const player={tag:'#PYQ',name:'Bruno',attacks:[{stars:2,destructionPercentage:70,defenderTag:'#YQL'}]}
const pastStart='20261001T070000.000Z',pastEnd='20261002T070000.000Z'
const futureEnd=new Date(Date.now()+86400000).toISOString()
const battle={state:'warEnded',teamSize:1,startTime:pastStart,endTime:pastEnd,attacksPerMember:2,clan:{tag:'#OTHER',members:[{tag:'#LGC',name:'Outro clã'}]},opponent:{tag:'#CLAN',attacks:1,members:[player]}}
test('Guerra encerrada cobra apenas zero e preserva ataques parciais no relatório',()=>{
 assert.deepEqual(battlePending({war:battle},'CLAN','Guerra',2),[])
 const war={...battle,opponent:{...battle.opponent,attacks:0,members:[{tag:'#PYQ',name:'Bruno'}]}}
 const items=battlePending({war},'CLAN','Guerra',2)
 assert.equal(items.length,1);assert.equal(items[0].used,0);assert.equal(items[0].ended,true)
 assert.equal(battlePending({war},'THIRD','Guerra').length,0)
 assert.equal(battleStatus({...battle,state:'preparation'},player,2).remaining,0)
 assert.equal(battleStatus(battle,null,2).label,'Não escalado')
})
test('Todos da CWL inclui reservas, participantes históricos e indisponibilidade sem inventar faltas',()=>{
 const group={clans:[{tag:'#CLAN',members:[{tag:'#QPC',name:'Reserva'}]}]}
 const rounds=[{round:1,war:battle},{round:2,error:'Indisponível'}]
 const rows=seasonRows(group,rounds,'CLAN')
 assert.equal(rows.length,2)
 assert.equal(rows.find(p=>p.tag==='#QPC').rounds[0].label,'Não escalado')
 assert.equal(rows.find(p=>p.tag==='#PYQ').rounds[1].remaining,0)
 assert.equal(rows.find(p=>p.tag==='#PYQ').rounds[1].label,'Falha na consulta')
})
test('CSV conserva clã e rodada, conta ataques parciais e neutraliza nomes como fórmulas',()=>{
 const war={...battle,opponent:{tag:'#CLAN',members:[{...player,name:'=HYPERLINK(1)'}]}}
 const csv=battleCSV({clanName:'Meu clã',clanTag:'CLAN',type:'CWL',season:'2026-10',rounds:[{round:3,war},{round:4,error:'Falha na API'}]})
 assert.ok(csv.startsWith('\uFEFF'))
 assert.ok(csv.includes("\"'=HYPERLINK(1)\""));assert.ok(csv.includes('"3","Encerrada"'))
 assert.ok(csv.includes('"1","2","1","Não realizados","2","70","#YQL"'))
 assert.ok(csv.includes('"4","Falha na API"'))
})
test('Mensagem individual diferencia prazo aberto, pede justificativa só para encerrados e mantém as contas separadas',()=>{
 const closed=battlePending({war:{...battle,opponent:{...battle.opponent,attacks:0,members:[{tag:'#PYQ',name:'Bruno'}]}}},'CLAN','Guerra',2)
 const active=battlePending({war:{...battle,state:'inWar',endTime:futureEnd}},'CLAN','CWL · rodada 2',2)
 const items=[...closed,...active,{...closed[0],tag:'#QPC',name:'Outra conta'}]
 const text=pendingMessage({clanName:'Meu clã',clanTag:'CLAN',items,playerTag:'#PYQ'})
 assert.ok(text.includes('Sem justificativa, ficará sob aviso.'));assert.ok(text.includes('O prazo ainda está aberto.'));assert.ok(!text.includes('Outra conta'))
 const reminder=pendingMessage({clanTag:'CLAN',items:active})
 assert.ok(!reminder.includes('ficará sob aviso'));assert.equal(pendingMessage({clanTag:'CLAN',items:[]}), '')
})
test('Avisos de guerra separam zero e um ataque sem incluir quem já cumpriu o limite',()=>{
 const war={...battle,state:'inWar',endTime:futureEnd,teamSize:3,opponent:{tag:'#CLAN',attacks:3,members:[{tag:'#ZERO',name:'Sem ataques'},player,{tag:'#FULL',name:'Concluído',attacks:[{},{}]}]}}
 const items=battlePending({war},'CLAN','Guerra',2)
 assert.deepEqual(filterBattleWarnings(items,'0').map(item=>item.tag),['#ZERO'])
 assert.deepEqual(filterBattleWarnings(items,'1').map(item=>item.tag),['#PYQ'])
 assert.equal(filterBattleWarnings(items).length,2)
 const completedSingle={tag:'#SINGLE',name:'Limite cumprido',used:1,limit:1,ended:true}
 assert.deepEqual(filterBattleWarnings([...items,completedSingle],'1').map(item=>item.tag),['#PYQ'])
 assert.equal(filterBattleWarnings(battlePending({war:{...war,attacksPerMember:1}},'CLAN','Guerra',2),'1').length,0)
 const closed=battlePending({war:{...war,state:'warEnded',endTime:pastEnd}},'CLAN','Guerra',2)
 const text=pendingMessage({clanTag:'CLAN',items:filterBattleWarnings(closed,'0')})
 assert.ok(text.includes('Sem ataques (#ZERO)'));assert.ok(text.includes('Sem justificativa, ficará sob aviso.'))
 assert.ok(!text.includes('Bruno'));assert.ok(!text.includes('Outro clã'));assert.ok(!text.includes('Concluído'))
})
test('Filtro de guerra não inventa faltas na preparação ou sem dados e mantém o lembrete durante a guerra',()=>{
 for(const state of ['preparation','notInWar',undefined]){
  assert.deepEqual(filterBattleWarnings(battlePending({war:{...battle,state}},'CLAN','Guerra',2),'1'),[])
 }
 assert.deepEqual(filterBattleWarnings(battlePending({},'CLAN','Guerra',2),'0'),[])
 const active=filterBattleWarnings(battlePending({war:{...battle,state:'inWar',endTime:futureEnd}},'CLAN','Guerra',2),'1')
 const text=pendingMessage({clanTag:'CLAN',items:active})
 assert.ok(text.includes('1/2 ataques usados'));assert.ok(text.includes('O prazo ainda está aberto.'))
 assert.ok(!text.includes('justificativa necessária'));assert.ok(!text.includes('ficará sob aviso'))
})
test('Raids considera bônus, exige conferência para ausência de ataques e evita limite inventado',()=>{
 const record={rosterDuringRaid:true,roster:[{tag:'#QPC',name:'Sem registro'}],season:{state:'ended',startTime:pastStart,endTime:pastEnd,totalAttacks:8,members:[{tag:'#PYQ',name:'Parcial',attacks:5,attackLimit:5,bonusAttackLimit:1},{tag:'#LGC',name:'Limite desconhecido',attacks:3}]}}
 const items=raidPending(record)
 assert.equal(items.length,2);assert.equal(items.find(p=>p.tag==='#PYQ').limit,6)
 assert.equal(items.find(p=>p.tag==='#QPC').verify,true)
 assert.ok(pendingMessage({clanTag:'CLAN',items}).includes('confirme a elegibilidade'))
})

test('CWL encerrada considera apenas zero, sem inventar faltas em reservas ou em respostas incompletas',()=>{
 const war={...battle,attacksPerMember:undefined,teamSize:2,opponent:{tag:'#CLAN',attacks:1,members:[{tag:'#QPC',name:'Sem ataque'},player]}}
 assert.deepEqual(battlePending({war},'CLAN','CWL',1).map(p=>p.tag),['#QPC'])
 for(const invalid of [
  {...war,state:'inWar',endTime:pastEnd},
  {...war,endTime:futureEnd},
  {...war,startTime:undefined},
  {...war,teamSize:3},
  {...war,opponent:{...war.opponent,attacks:2}},
  {...war,opponent:{...war.opponent,attacks:undefined}},
 ])assert.deepEqual(battlePending({war:invalid},'CLAN','CWL',1),[])
})

test('Raids exige confirmação final e conta somente os ataques e o bônus disponíveis',()=>{
 const season={state:'ended',startTime:pastStart,endTime:pastEnd,totalAttacks:13,members:[
  {tag:'#QPC',name:'Parcial',attacks:3,attackLimit:5},
  {tag:'#PYQ',name:'Bônus pendente',attacks:5,attackLimit:5,bonusAttackLimit:1},
  {tag:'#LGC',name:'Concluiu sem bônus',attacks:5,attackLimit:5},
 ]}
 const record={rosterDuringRaid:true,roster:[{tag:'#NONE',name:'Sem ataques'}],season}
 assert.deepEqual(raidPending(record).map(p=>[p.name,p.used,p.limit]),[['Sem ataques',0,null],['Parcial',3,5],['Bônus pendente',5,6]])
 for(const invalid of [
  {...season,state:'ongoing'},
  {...season,state:undefined},
  {...season,startTime:undefined},
  {...season,endTime:futureEnd},
  {...season,totalAttacks:undefined},
  {...season,totalAttacks:14},
  {...season,members:undefined},
  {...season,members:season.members.map((m,i)=>i?m:{...m,attacks:undefined})},
 ])assert.deepEqual(raidPending({...record,season:invalid}),[])
})
