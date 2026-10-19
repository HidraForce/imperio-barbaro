import test from 'node:test'
import assert from 'node:assert/strict'
import { contactPhoneIndex, messagePhone, playerMessageLabel, contactCoverageLabel } from './contact-numbers.js'
import { pendingMessage } from './battle-report.js'
import { moderationMessage, punishmentListMessage } from './moderation.js'

const contacts=[{phone:'+55 (11) 99999-1234',notes:'Private notes',accounts:[{playerTag:'#PYQ',playerName:'Same name',clanId:'1'},{playerTag:'#QPC',playerName:'Second account',clanId:'2'}]},{phone:'',accounts:[{playerTag:'#LGC',playerName:'Same name',clanId:'2'}]}]
test('Phone matches use tags across both clans and all accounts of one person, never nickname similarity',()=>{
  const phones=contactPhoneIndex(contacts)
  assert.deepEqual(phones,{'#PYQ':'5511999991234','#QPC':'5511999991234'})
  assert.equal(playerMessageLabel('Same name','pyq',phones),'Same name @11999991234 (pyq)')
  assert.equal(playerMessageLabel('Same name','#LGC',phones),'Same name (#LGC)')
  assert.equal(playerMessageLabel('No contact','#PPPP',phones),'No contact (#PPPP)')
  assert.deepEqual(contactPhoneIndex([{phone:'invalid @123',accounts:[{playerTag:'#PYQ'}]}]),{})
})
test('Coverage distinguishes registered, missing and unavailable without exposing phones',()=>{
  assert.equal(contactCoverageLabel('pyq',{'#PYQ':true}),'Com número')
  assert.equal(contactCoverageLabel('#PYQ',{'#PYQ':false}),'Sem número')
  assert.equal(contactCoverageLabel('#LGC',{}),'Sem número')
  assert.equal(contactCoverageLabel('#LGC',null),'Não conferido')
})
test('Pending and punishment messages add @number only beside the matching nickname and omit contact notes',()=>{
  const phones=contactPhoneIndex(contacts)
  const items=[{tag:'#PYQ',name:'Main',activity:'Guerra',used:0,limit:2,ended:true},{tag:'#QPC',name:'Second',activity:'CWL',used:0,limit:1,ended:true},{tag:'#LGC',name:'No phone',activity:'Raids',used:0,limit:5,ended:true}]
  const text=pendingMessage({clanTag:'CLAN',clanName:'Test',items,phones})
  assert.match(text,/Main @11999991234 \(#PYQ\)/)
  assert.match(text,/Second @11999991234 \(#QPC\)/)
  assert.match(text,/No phone \(#LGC\)/)
  assert.ok(!text.includes('Private notes'))
  const punishment={kind:'punishment',status:'active',playerTag:'#QPC',playerName:'Second',reason:'Zero attacks',action:'One war out',notes:'Private notes'}
  assert.match(moderationMessage(punishment,'Clan 2',phones),/Second @11999991234/)
  assert.match(punishmentListMessage([punishment],'Clan 2','',phones),/Second @11999991234/)
  const without=pendingMessage({clanTag:'CLAN',items,phones:{}})
  assert.ok(!without.includes('@11999991234'))
})

test('Message format removes only the Brazilian country code and preserves stored phones and national DDD 55',()=>{
  for(const [phone,expected] of [['+55 (11) 99999-1234','11999991234'],['551133331234','1133331234'],['11999991234','11999991234'],['55999991234','55999991234'],['+351912345678','351912345678'],['',''],['123','']])assert.equal(messagePhone(phone),expected)
  const original=structuredClone(contacts)
  const phones=contactPhoneIndex(contacts)
  assert.match(playerMessageLabel('Second','#QPC',phones),/@11999991234/)
  assert.equal(phones['#QPC'],'5511999991234')
  assert.deepEqual(contacts,original)
})
