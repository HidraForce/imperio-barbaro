import { accountTag } from './contact-numbers.js'

export function clanPresence(tag,members){
  if(!Array.isArray(members))return null
  return members.some(member=>accountTag(member.tag)===accountTag(tag))
}
export function withClanPresence(items,members){
  return items.map(item=>({...item,current:clanPresence(item.tag,members)}))
}
