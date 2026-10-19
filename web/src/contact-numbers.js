export const accountTag = value => '#'+String(value || '').trim().replace(/^#/,'').toUpperCase()
export function contactPhoneIndex(contacts=[]) {
  const index={}
  for(const contact of contacts){
    const phone=String(contact.phone||'').replace(/\D/g,'')
    if(!/^\d{7,15}$/.test(phone))continue
    for(const account of contact.accounts||[])if(account.playerTag)index[accountTag(account.playerTag)]=phone
  }
  return index
}
export function messagePhone(value){
  const phone=String(value||'').replace(/\D/g,'')
  if(!/^\d{7,15}$/.test(phone))return ''
  return /^55\d{10,11}$/.test(phone)?phone.slice(2):phone
}
export function playerMessageLabel(name,tag,phones={},current=null){
  const phone=messagePhone(phones[accountTag(tag)])
  return `${name}${phone?' @'+phone:''} (${tag})${current===false?' [FORA DO CLÃ ATUAL]':''}`
}
export function contactCoverageLabel(tag,coverage){
  return coverage==null?'Não conferido':coverage[accountTag(tag)]?'Com número':'Sem número'
}
