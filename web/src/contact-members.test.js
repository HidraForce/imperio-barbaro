import test from 'node:test'
import assert from 'node:assert/strict'
import { loadContactMembers, contactMemberOptions, accountMemberOptions, selectContactMember } from './contact-members.js'
import { createContactVault } from './contact-vault.js'

const clans = [{id:'1',configured:true},{id:'2',configured:true},{id:'3',configured:false}]
const lists = {1:[{tag:'#PYQ',name:'Principal'}],2:[{tag:'#QPC',name:'Secundária'},{tag:'#PYQ',name:'Cache repetido'}]}

test('Reuses loaded roster and queries only missing clans through the cached clan endpoint', async () => {
  const calls = []
  const result = await loadContactMembers(clans, {1:lists[1]}, async path => {calls.push(path);return {memberList:lists[2]}})
  assert.deepEqual(calls, ['/api/clan?clan=2'])
  assert.deepEqual(result.lists, lists)
  assert.deepEqual(result.failed, [])
  assert.deepEqual(contactMemberOptions(clans,result.lists), [{playerTag:'#PYQ',playerName:'Principal',clanId:'1'},{playerTag:'#QPC',playerName:'Secundária',clanId:'2'}])
})

test('One failed clan does not discard the available roster or fabricate members', async () => {
  const result = await loadContactMembers(clans, {1:lists[1]}, async () => {throw Error('Unavailable')})
  assert.deepEqual(result.lists, {1:lists[1]})
  assert.deepEqual(result.failed, ['2'])
})

test('Prevents selecting an account twice or linking an account owned by another contact', () => {
  const account = {playerTag:'',playerName:'',clanId:''}
  const form = {id:7,accounts:[account,{playerTag:'#QPC'}]}
  const contacts = [{id:8,accounts:[{playerTag:'#PYQ'}]}]
  const options = accountMemberOptions(contactMemberOptions(clans,lists),contacts,form,account)
  assert.ok(options.every(option => option.disabled))
  assert.equal(selectContactMember(account,'#PYQ',options),false)
  assert.equal(account.playerTag,'')
  const editing = {id:8,accounts:[{playerTag:'#PYQ',playerName:'Principal',clanId:'1'}]}
  assert.equal(accountMemberOptions(contactMemberOptions(clans,lists),contacts,editing,editing.accounts[0])[0].disabled,false)
})

test('Keeps a saved account selectable when it has left the cached rosters', () => {
  const account = {playerTag:'#LGC',playerName:'Conta antiga',clanId:'2'}
  const options = accountMemberOptions(contactMemberOptions(clans,lists),[],{id:1,accounts:[account]},account)
  const saved = options.find(option => option.playerTag==='#LGC')
  assert.equal(saved.saved,true)
  assert.equal(saved.playerName,'Conta antiga')
  assert.equal(saved.disabled,false)
})

test('Saves multiple selected nicknames from both clans as one person', async () => {
  let saved
  const vault = createContactVault(async (path,options) => {
    if(path.endsWith('/unlock'))return Response.json({token:'test-token',expiresAt:new Date(Date.now()+600000).toISOString()})
    if(path.endsWith('/session'))return new Response(null,{status:204})
    if(options.method==='POST'){saved=JSON.parse(options.body);return new Response(null,{status:204})}
    return Response.json([])
  })
  vault.password.value='test-password'
  await vault.unlock()
  try {
    vault.startNew();vault.form.value.realName='Uma pessoa'
    const options=contactMemberOptions(clans,lists)
    assert.equal(selectContactMember(vault.form.value.accounts[0],'#PYQ',options),true)
    vault.addAccount()
    assert.equal(selectContactMember(vault.form.value.accounts[1],'#QPC',options),true)
    await vault.save()
    assert.equal(saved.realName,'Uma pessoa')
    assert.deepEqual(saved.accounts,options)
  } finally {vault.lock()}
})
