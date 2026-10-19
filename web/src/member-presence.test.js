import test from 'node:test'
import assert from 'node:assert/strict'
import { clanPresence, withClanPresence } from './member-presence.js'
import { buildAttackOverview } from './attack-overview.js'
import { pendingMessage } from './battle-report.js'
import { moderationMessage, punishmentListMessage } from './moderation.js'

const members=[{tag:'#PYQ',name:'Atual'}]
const items=[{tag:'#PYQ',name:'Atual',activity:'Guerra',used:0,limit:2,ended:true},{tag:'#QPC',name:'Saiu',activity:'Guerra',used:0,limit:2,ended:true},{tag:'#QPC',name:'Saiu',activity:'CWL',used:0,limit:1,ended:false}]
test('Presence uses the current clan roster by tag, including a confirmed empty roster, and never infers departure from unavailable data',()=>{
  assert.equal(clanPresence('pyq',members),true)
  assert.equal(clanPresence('#QPC',members),false)
  assert.equal(clanPresence('#PYQ',[]),false)
  assert.equal(clanPresence('#PYQ',undefined),null)
  assert.equal(clanPresence('#QPC',[{tag:'#QPC',name:'Mesmo jogador no outro clã'}]),true)
  const marked=withClanPresence(items,members)
  assert.deepEqual(marked.map(item=>item.current),[true,false,false])
  assert.equal(items[0].current,undefined)
})
test('Historical war participants remain visible; loading and failed roster are not treated as departures',()=>{
  const war={state:'warEnded',clan:{members:items.slice(0,2)}}
  const rows=buildAttackOverview({members,war})
  assert.equal(rows.find(row=>row.tag==='#PYQ').current,true)
  assert.equal(rows.find(row=>row.tag==='#QPC').current,false)
  assert.equal(rows.length,2)
  for(const input of [{war},{members,war,loading:true}])assert.ok(buildAttackOverview(input).every(row=>row.current===null))
})
test('Attack messages isolate former members in a history section in both contact formats, without threatening absent members',()=>{
  for(const phones of [{},{'#QPC':'5511999991234'}]){
    const marked=withClanPresence(items,members)
    const text=pendingMessage({clanName:'Clan',clanTag:'CLAN',items:marked,phones})
    const [current,history]=text.split('*Fora do clã atual — histórico de ataques*')
    assert.ok(current.includes('Atual'))
    assert.ok(!current.includes('Saiu'))
    assert.ok(history.includes('Saiu')&&history.includes('[FORA DO CLÃ ATUAL]'))
    assert.ok(!history.includes('ficará sob aviso'))
    const individual=pendingMessage({clanTag:'CLAN',items:marked,playerTag:'#QPC',phones})
    assert.ok(!individual.includes('ficará sob aviso'))
    assert.ok(!individual.includes('Realize os ataques'))
    assert.equal(individual.includes('@11999991234'),Object.keys(phones).length>0)
  }
  const unknown=pendingMessage({items:withClanPresence(items,undefined)})
  assert.ok(!unknown.includes('FORA DO CLÃ ATUAL'))
  assert.ok(unknown.includes('Vínculo não conferido'))
  assert.ok(!unknown.includes('ficará sob aviso'))
  assert.ok(!unknown.includes('Realize os ataques'))
})
test('Individual discipline notices and punishment lists clearly identify former members without changing saved decisions',()=>{
  const record={kind:'warning',status:'active',playerName:'Saiu',playerTag:'#QPC',reason:'Ocorrência histórica',notes:'Private notes'}
  const text=moderationMessage(record,'Clan',{'#QPC':'5511999991234'},false)
  assert.match(text,/Saiu @11999991234.*FORA DO CLÃ ATUAL/)
  assert.ok(!text.includes('Uma segunda ocorrência'))
  assert.ok(!text.includes(record.notes))
  const punishment={...record,kind:'punishment',action:'Revisar histórico'}
  assert.match(punishmentListMessage([punishment],'Clan','',{},members),/FORA DO CLÃ ATUAL/)
  assert.ok(!punishmentListMessage([punishment],'Clan').includes('FORA DO CLÃ ATUAL'))
  assert.equal(record.status,'active')
})
