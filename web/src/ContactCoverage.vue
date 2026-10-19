<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { adminAPI } from './admin-api.js'
import { loadContactMembers } from './contact-members.js'
import { contactCoverageLabel } from './contact-numbers.js'
const props=defineProps({clans:Array,members:Array,selectedClan:String})
const emit=defineEmits(['coverage'])
const coverage=ref(null),lists=ref({}),failed=ref([]),busy=ref(false),error=ref(''),filter=ref('all')
let mounted=true
const summaries=computed(()=>props.clans.filter(c=>c.configured).map(clan=>{
  const members=lists.value[clan.id]
  const available=coverage.value!==null&&Array.isArray(members)
  const withPhone=available?members.filter(m=>contactCoverageLabel(m.tag,coverage.value)==='Com número').length:0
  return {...clan,members,available,withPhone,withoutPhone:available?members.length-withPhone:0}
}))
const rows=computed(()=>summaries.value.flatMap(clan=>(clan.members||[]).map(member=>({...member,clanId:clan.id,status:contactCoverageLabel(member.tag,coverage.value)}))).filter(member=>filter.value==='all'||member.status===(filter.value==='with'?'Com número':'Sem número')))
async function load(){
  if(busy.value)return
  busy.value=true;error.value='';coverage.value=null;emit('coverage',null)
  const results=await Promise.allSettled([adminAPI('/api/contacts/coverage'),loadContactMembers(props.clans,{...(props.members?{[props.selectedClan]:props.members}:{})})])
  if(!mounted)return
  if(results[0].status==='fulfilled'){coverage.value=results[0].value.items;emit('coverage',coverage.value)}else error.value=results[0].reason.message
  if(results[1].status==='fulfilled'){lists.value=results[1].value.lists;failed.value=results[1].value.failed}else{lists.value={};error.value=results[1].reason.message}
  busy.value=false
}
onMounted(load)
onUnmounted(()=>{mounted=false})
</script>
<template>
  <section class="contact-coverage">
    <div class="section"><h2>Números cadastrados · dois clãs</h2><button class="secondary" :disabled="busy" @click="load">Atualizar situação dos contatos</button></div>
    <p class="report-note">Contagem por conta do jogo. A mesma pessoa pode ter nicknames nos dois clãs. Esta lista mostra se existe número, sem expor os telefones.</p>
    <p v-if="busy" role="status">Conferindo membros e contatos…</p><p v-if="error" class="notice error" role="alert">{{ error }}</p>
    <p v-for="id in failed" :key="id" class="notice error">Lista do clã {{ id }} indisponível. Não é possível confirmar a cobertura desse clã.</p>
    <div class="contact-coverage-cards"><article v-for="clan in summaries" :key="clan.id"><strong>Clã {{ clan.id }} · #{{ clan.tag }}</strong><p v-if="clan.available">{{ clan.withPhone }} com número · {{ clan.withoutPhone }} sem número · {{ clan.members.length }} contas</p><p v-else>Situação não conferida</p></article></div>
    <details><summary>Ver quem tem ou não número nos dois clãs</summary><label class="message-recipient">Filtrar números cadastrados<select v-model="filter"><option value="all">Todos</option><option value="without">Sem número</option><option value="with">Com número</option></select></label><div class="table-wrap"><table><thead><tr><th>JOGADOR</th><th>CLÃ</th><th>CONTATO</th></tr></thead><tbody><tr v-for="member in rows" :key="member.clanId+member.tag"><td><b>{{ member.name }}</b><small>{{ member.tag }}</small></td><td>Clã {{ member.clanId }}</td><td>{{ member.status }}</td></tr><tr v-if="!rows.length"><td colspan="3" class="empty">{{ busy?'Consultando…':'Nenhum membro neste filtro.' }}</td></tr></tbody></table></div></details>
  </section>
</template>
