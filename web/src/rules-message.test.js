import test from 'node:test'
import assert from 'node:assert/strict'
import { rulesMessage } from './rules-message.js'

const document = () => ({revision:4,general:{title:'Respeito',penalty:'Consulte a liderança.',items:['Sem insultos.']},clans:[{id:'1',anchor:'regras-principal',name:'Principal',warSummary:'Dois ataques',inactivitySummary:'Avisar ausências',documentUrl:'https://example.com/rules.pdf',topics:[{id:'regras-principal-war',title:'Guerra',items:['Ataque o espelho.']}]},{id:'2',anchor:'regras-secundario',name:'Secundário',warSummary:'Um ataque',inactivitySummary:'Participar',documentUrl:'',topics:[{id:'regras-secundario-war',title:'Liga',items:['Confirmar escalação.']}]}]})

test('Rules message includes published revision, selected clan, general rules and public document links',()=>{
  const text=rulesMessage(document(),'1')
  assert.match(text,/Versão publicada: 4/)
  assert.match(text,/Sem insultos/)
  assert.match(text,/Principal/)
  assert.match(text,/https:\/\/example.com\/rules.pdf/)
  assert.match(text,/https:\/\/imperiobarbaro.com.br\/#regras-principal/)
  assert.ok(!text.includes('Secundário'))
  assert.ok(!text.includes('Atualização das regras'))
  assert.equal(rulesMessage(null),'')
})

test('Publication generates actual changed rule text and identifies added or removed topics',()=>{
  const before=document(),after=structuredClone(before)
  after.revision++
  after.clans[0].topics[0].items=['Ataque o alvo combinado com a liderança.']
  after.clans[0].topics.push({id:'regras-principal-raids',title:'Raids',items:['Use os ataques disponíveis.']})
  after.clans[1].topics=[]
  const text=rulesMessage(after,'all',before)
  assert.match(text,/Atualização das regras/)
  assert.match(text,/Tópico atualizado: Guerra/)
  assert.match(text,/Ataque o alvo combinado/)
  assert.match(text,/Novo tópico: Raids/)
  assert.match(text,/Tópico removido: Liga/)
  assert.ok(!text.includes('Ataque o espelho.'))
  assert.equal(before.clans[0].topics[0].items[0],'Ataque o espelho.')
})

test('General-only announcement excludes clan changes and unrelated clan announcements remain current summaries',()=>{
  const before=document(),after=structuredClone(before)
  after.general.items=['Nova regra geral publicada.']
  after.clans[1].warSummary='Nova estratégia secundária'
  const text=rulesMessage(after,'general',before)
  assert.match(text,/Nova regra geral publicada/)
  assert.ok(!text.includes('Nova estratégia secundária'))
  const clanOnly=rulesMessage({...before,clans:after.clans},'1',before)
  assert.ok(!clanOnly.includes('Atualização das regras'))
  assert.ok(!clanOnly.includes('Secundário'))
})

test('Changed summaries, removed PDFs and reordered topics are explained without sharing obsolete links',()=>{
  const before=document(),after=structuredClone(before)
  after.clans[0].warSummary='Estratégia atualizada'
  after.clans[0].documentUrl=''
  let text=rulesMessage(after,'1',before)
  assert.match(text,/Estratégia atualizada/)
  assert.match(text,/regulamento anterior foi removido/)
  assert.ok(!text.includes('https://example.com/rules.pdf'))
  before.clans[0].topics.push({id:'regras-principal-extra',title:'Outro',items:['Texto.']})
  const reordered=structuredClone(before)
  reordered.clans[0].topics.reverse()
  text=rulesMessage(reordered,'1',before)
  assert.match(text,/ordem dos tópicos foi atualizada/)
})
