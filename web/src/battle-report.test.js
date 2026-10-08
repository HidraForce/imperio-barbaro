import test from 'node:test'
import assert from 'node:assert/strict'
import { battleStatus, seasonRows, battleCSV, battlePending, raidPending, pendingMessage } from './battle-report.js'
const player={tag:'#PYQ',name:'Bruno',attacks:[{stars:2,destructionPercentage:70,defenderTag:'#YQL'}]}
const battle={state:'warEnded',attacksPerMember:2,clan:{tag:'#OTHER',members:[{tag:'#LGC',name:'Outro clã'}]},opponent:{tag:'#CLAN',members:[player]}}
test('Relatório encontra o clã em ambos os lados e cobra também ataques parciais após o encerramento',()=>{
 const items=battlePending({war:battle},'CLAN','Guerra',2)
 assert.equal(items.length,1);assert.equal(items[0].tag,'#PYQ');assert.equal(items[0].used,1);assert.equal(items[0].ended,true)
 assert.equal(battlePending({war:battle},'THIRD','Guerra').length,0)
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
 const closed=battlePending({war:battle},'CLAN','Guerra',2)
 const active=battlePending({war:{...battle,state:'inWar'}},'CLAN','CWL · rodada 2',2)
 const items=[...closed,...active,{...closed[0],tag:'#QPC',name:'Outra conta'}]
 const text=pendingMessage({clanName:'Meu clã',clanTag:'CLAN',items,playerTag:'#PYQ'})
 assert.ok(text.includes('Sem justificativa, ficará sob aviso.'));assert.ok(text.includes('O prazo ainda está aberto.'));assert.ok(!text.includes('Outra conta'))
 const reminder=pendingMessage({clanTag:'CLAN',items:active})
 assert.ok(!reminder.includes('ficará sob aviso'));assert.equal(pendingMessage({clanTag:'CLAN',items:[]}), '')
})
test('Incursões considera bônus, exige conferência para ausência de ataques e evita limite inventado',()=>{
 const record={rosterDuringRaid:true,roster:[{tag:'#QPC',name:'Sem registro'}],season:{state:'ended',startTime:'20261002T070000.000Z',members:[{tag:'#PYQ',name:'Parcial',attacks:5,attackLimit:5,bonusAttackLimit:1},{tag:'#LGC',name:'Limite desconhecido',attacks:3}]}}
 const items=raidPending(record)
 assert.equal(items.length,2);assert.equal(items.find(p=>p.tag==='#PYQ').limit,6)
 assert.equal(items.find(p=>p.tag==='#QPC').verify,true)
 assert.ok(pendingMessage({clanTag:'CLAN',items}).includes('confirme a elegibilidade'))
})
