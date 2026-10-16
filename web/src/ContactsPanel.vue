<script setup>
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { createContactVault } from './contact-vault.js'
import { loadContactMembers, contactMemberOptions, accountMemberOptions, selectContactMember } from './contact-members.js'
import UiIcon from './UiIcon.vue'

const props = defineProps({ configured: Boolean, loading: Boolean, clans: {type:Array,default:()=>[]}, members: {type:Array,default:()=>[]}, selectedClan: String })
const { contacts, form, password, unlocked, busy, error, showForm, unlock, lock, save, remove, startNew, edit, addAccount, removeAccount, merge } = createContactVault()
const mergeTarget=ref(''), mergeSource=ref('')
const memberLists=ref({}), membersLoading=ref(false), membersError=ref(''), manualAccounts=ref(new Set())
const memberChoices=computed(()=>contactMemberOptions(props.clans,memberLists.value))
let memberRequest=0
watch(unlocked,value=>{if(!value){mergeTarget.value='';mergeSource.value='';memberRequest++;memberLists.value={};membersError.value='';membersLoading.value=false;manualAccounts.value.clear()}})
watch(form,()=>manualAccounts.value.clear())
watch(showForm,value=>{if(value)loadMembers()})
function optionsFor(account){return accountMemberOptions(memberChoices.value,contacts.value,form.value,account)}
async function loadMembers(refresh=false){
  if(!unlocked.value)return
  const request=++memberRequest;membersLoading.value=true;membersError.value=''
  const cached=refresh?{}:{...memberLists.value,...(props.members.length?{[props.selectedClan]:props.members}:{})}
  const result=await loadContactMembers(props.clans,cached)
  if(request!==memberRequest||!unlocked.value)return
  memberLists.value={...memberLists.value,...result.lists};membersLoading.value=false
  if(result.failed.length)membersError.value='Não foi possível consultar os membros do clã '+result.failed.join(', ')+'. Você pode preencher a conta manualmente ou tentar atualizar a lista.'
}
function chooseAccount(account,event){
  if(event.target.value==='manual'){manualAccounts.value.add(account);return}
  manualAccounts.value.delete(account)
  if(!event.target.value){Object.assign(account,{playerTag:'',playerName:'',clanId:''});return}
  selectContactMember(account,event.target.value,optionsFor(account))
}
function unlinkAccount(index){manualAccounts.value.delete(form.value.accounts[index]);removeAccount(index)}
function saveContact(){
  for(const account of form.value.accounts)if(!manualAccounts.value.has(account))selectContactMember(account,account.playerTag,optionsFor(account))
  return save()
}
function clanLabel(id){const clan=props.clans.find(c=>c.id===id);return id?'Clã '+id+(clan?.tag?' · #'+clan.tag:''):'Clã não informado'}
const whats = phone => 'https://wa.me/' + phone.replace(/\D/g, '')
function hidden() { if (document.hidden) lock() }
function leaving() { lock() }
watch(() => props.loading, value => { if (value) lock() })
onMounted(() => { document.addEventListener('visibilitychange', hidden); window.addEventListener('pagehide', leaving) })
onUnmounted(() => { document.removeEventListener('visibilitychange', hidden); window.removeEventListener('pagehide', leaving); lock() })
</script>
<template>
  <p v-if="loading" class="loading">Carregando configuração dos contatos…</p>
  <section v-else-if="!unlocked" class="contact-lock">
    <UiIcon name="Shield"/>
    <small>ACESSO RESTRITO</small>
    <h2>Contatos protegidos</h2>
    <p>Informe a senha exclusiva dos contatos. O acesso dura até 10 minutos e é bloqueado ao sair desta área ou ocultar a aba.</p>
    <div v-if="!configured" class="notice">A proteção ainda não foi configurada. Defina <code>CONTACTS_PASSWORD</code> no servidor para habilitar esta área.</div>
    <form v-else @submit.prevent="unlock">
      <label for="contacts-password">Senha dos contatos</label>
      <input id="contacts-password" v-model="password" type="password" autocomplete="off" maxlength="1024" :disabled="busy" required>
      <button class="primary" :disabled="busy">{{ busy?'Desbloqueando…':'Desbloquear contatos' }}</button>
    </form>
    <p v-if="error" class="notice error" role="alert">{{ error }}</p>
  </section>
  <template v-else>
    <div class="section contact-heading"><div><small>LISTA PRIVADA</small><h2>Pessoas e contas <em>{{ contacts.length }}</em></h2><p>Um contato por pessoa. Vincule todas as contas dela, mesmo quando estiverem nos dois clãs.</p></div><div class="heading-actions"><button class="secondary" @click="lock()">Bloquear contatos</button><button class="primary" :disabled="busy" @click="startNew">＋ Adicionar contato</button></div></div>
    <p v-if="error" class="notice error" role="alert">{{ error }}</p>
    <p v-if="busy" class="loading" role="status">Carregando contatos…</p>
    <form v-if="showForm" class="form person-form" @submit.prevent="saveContact">
      <h3>{{ form.id?'Editar pessoa e contas':'Nova pessoa' }}</h3>
      <label>Nome da pessoa ou apelido<input v-model="form.realName" placeholder="Como você identifica esta pessoa" maxlength="100" :disabled="busy" required></label>
      <label>WhatsApp da pessoa<input v-model="form.phone" placeholder="+55 11 99999-0000" inputmode="tel" maxlength="32" :disabled="busy"></label>
      <fieldset class="account-fields"><legend>Contas desta pessoa · {{ form.accounts.length }}</legend>
        <p class="report-note">Selecione os nicknames das listas em cache dos dois clãs. A tag e o clã são preenchidos juntos. Use “Vincular outro nickname” para adicionar mais contas à mesma pessoa.</p>
        <p v-if="membersLoading" class="loading" role="status">Consultando membros…</p>
        <p v-if="membersError" class="notice error" role="alert">{{ membersError }}</p>
        <button type="button" class="secondary" :disabled="busy||membersLoading" @click="loadMembers(true)">↻ Atualizar lista de membros</button>
        <div v-for="(account,index) in form.accounts" :key="index" class="account-row">
          <div><label :for="'contact-account-'+index">Nickname / conta {{ index+1 }}</label><select :id="'contact-account-'+index" :value="manualAccounts.has(account)?'manual':account.playerTag" :disabled="busy" required @change="chooseAccount(account,$event)"><option value="">Selecione um membro</option><option v-for="member in optionsFor(account)" :key="member.playerTag" :value="member.playerTag" :disabled="member.disabled">{{ member.playerName }} · {{ member.playerTag }} · {{ member.saved?'Conta salva fora da lista':clanLabel(member.clanId) }}{{ member.unavailable?' · '+member.unavailable:'' }}</option><option value="manual">Conta fora da lista · preencher manualmente</option></select><label v-if="manualAccounts.has(account)">Nome no jogo<input v-model="account.playerName" maxlength="100" :disabled="busy" required></label></div>
          <label>Tag da conta<input v-model="account.playerTag" placeholder="#P0Y8LQ" maxlength="16" :disabled="busy" :readonly="!manualAccounts.has(account)" required></label>
          <label>Clã da conta<select v-model="account.clanId" :disabled="busy||!manualAccounts.has(account)"><option value="">Clã não informado</option><option v-for="clan in clans" :key="clan.id" :value="clan.id">{{ clanLabel(clan.id) }}</option></select></label>
          <button type="button" class="secondary" :disabled="busy||form.accounts.length===1" :aria-label="'Remover conta '+(index+1)+' deste cadastro'" @click="unlinkAccount(index)">Remover</button>
        </div>
        <button type="button" class="secondary" :disabled="busy||form.accounts.length>=20" @click="addAccount">＋ Vincular outro nickname</button>
      </fieldset>
      <label class="person-notes">Observações<textarea v-model="form.notes" maxlength="1000" :disabled="busy" rows="3" placeholder="Informações sobre a pessoa e suas contas"></textarea></label>
      <div class="person-form-actions"><button class="primary" :disabled="busy">Salvar pessoa e contas</button><button type="button" class="secondary" :disabled="busy" @click="showForm=false">Cancelar</button></div>
    </form>
    <details v-if="contacts.length>1&&!showForm" class="contact-merge"><summary>Reunir cadastros da mesma pessoa</summary><p>As contas do segundo contato serão vinculadas ao primeiro. Telefones diferentes e observações serão preservados nas observações do contato mantido.</p><div class="merge-controls"><label>Contato que será mantido<select v-model="mergeTarget" :disabled="busy"><option value="">Selecione uma pessoa</option><option v-for="c in contacts" :key="c.id" :value="c.id">{{ c.realName }} · {{ c.accounts.length }} conta(s)</option></select></label><label>Outro cadastro da mesma pessoa<select v-model="mergeSource" :disabled="busy"><option value="">Selecione o outro cadastro</option><option v-for="c in contacts.filter(c=>c.id!==Number(mergeTarget))" :key="c.id" :value="c.id">{{ c.realName }} · {{ c.accounts.length }} conta(s)</option></select></label><button class="secondary" :disabled="busy||!mergeTarget||!mergeSource||Number(mergeTarget)===Number(mergeSource)" @click="merge(mergeTarget,mergeSource);mergeTarget='';mergeSource=''">Reunir contas</button></div></details>
    <section class="contacts"><article v-for="c in contacts" :key="c.id"><button class="delete" :disabled="busy" :aria-label="'Excluir pessoa '+c.realName+' e seus vínculos de contas'" @click="remove(c.id)">×</button><h3>{{ c.realName }}</h3><p>{{ c.accounts.length }} {{ c.accounts.length===1?'conta vinculada':'contas vinculadas' }}</p><ul class="contact-account-list"><li v-for="account in c.accounts" :key="account.playerTag"><b>{{ account.playerName }}</b><span>{{ account.playerTag }}</span><small :class="'account-clan-'+account.clanId">{{ clanLabel(account.clanId) }}</small></li></ul><p v-if="c.notes" class="contact-notes">{{ c.notes }}</p><a v-if="c.phone" :href="whats(c.phone)" target="_blank" rel="noreferrer">WhatsApp da pessoa ↗</a><small v-else>Nenhum telefone salvo</small><button class="secondary contact-edit" :disabled="busy" @click="edit(c)">Editar pessoa e contas</button></article><div v-if="!contacts.length&&!busy" class="empty">Nenhuma pessoa cadastrada. Crie um contato e vincule suas contas dos dois clãs.</div></section>

  </template>
</template>
