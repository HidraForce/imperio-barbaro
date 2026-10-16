const home = 'https://imperiobarbaro.com.br/'
const same = (a, b) => JSON.stringify(a) === JSON.stringify(b)
function generalLines(general){return [`*${general.title}*`,...general.items.map(item=>'• '+item),...(general.penalty?[general.penalty]:[]),home+'#regras-gerais']}
function clanSummary(clan){return [`*${clan.name}*`,'Guerras: '+clan.warSummary,'Inatividade: '+clan.inactivitySummary,...(clan.documentUrl?['Regulamento: '+clan.documentUrl]:[]),'Regras completas: '+home+'#'+clan.anchor]}

export function rulesMessage(document, clanId='all', previous=null){
  if(!document)return ''
  const clans=clanId==='general'?[]:document.clans.filter(clan=>clanId==='all'||clan.id===clanId)
  const changes=[]
  if(previous){
    if(!same(document.general,previous.general))changes.push(...generalLines(document.general),'')
    for(const clan of clans){
      const old=previous.clans.find(item=>item.id===clan.id)
      if(!old){changes.push(...clanSummary(clan),'');continue}
      const details=[]
      if(clan.name!==old.name)details.push('Nome atualizado: '+clan.name)
      if(clan.warSummary!==old.warSummary)details.push('Guerras: '+clan.warSummary)
      if(clan.inactivitySummary!==old.inactivitySummary)details.push('Inatividade: '+clan.inactivitySummary)
      if(clan.documentUrl!==old.documentUrl)details.push(clan.documentUrl?'Novo regulamento: '+clan.documentUrl:'O link do regulamento anterior foi removido. Consulte as regras no site.')
      for(const topic of clan.topics){
        const before=old.topics.find(item=>item.id===topic.id)
        if(!same(topic,before))details.push(`*${before?'Tópico atualizado':'Novo tópico'}: ${topic.title}*`,...topic.items.map(item=>'• '+item),home+'#'+topic.id,'')
      }
      for(const topic of old.topics)if(!clan.topics.some(item=>item.id===topic.id))details.push('Tópico removido: '+topic.title)
      if(!details.length&&!same(clan.topics.map(t=>t.id),old.topics.map(t=>t.id)))details.push('A ordem dos tópicos foi atualizada.')
      if(details.length)changes.push(`*${clan.name}*`,...details,'Regras completas: '+home+'#'+clan.anchor,'')
    }
  }
  const updated=changes.length>0
  const lines=[`*Império Bárbaro · ${updated?'Atualização das regras':'Regras da comunidade'}*`,`Versão publicada: ${document.revision}`,'',updated?'Pessoal, estas são as alterações publicadas. Leiam e sigam as orientações atualizadas:':'Pessoal, confiram as regras publicadas e sigam as orientações do seu clã:','']
  if(updated)lines.push(...changes)
  else{
    lines.push(...generalLines(document.general),'')
    for(const clan of clans)lines.push(...clanSummary(clan),'')
  }
  lines.push('Em caso de dúvida, procure a liderança antes de tomar qualquer medida.')
  return lines.join('\n')
}
