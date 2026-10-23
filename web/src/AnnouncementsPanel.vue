<script setup>
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { adminAPI } from './admin-api.js'
import { createAnnouncementVault } from './announcement-vault.js'
const props=defineProps({clanId:String,war:Object,warLoading:Boolean})
const vault=createAnnouncementVault('custom','X-Notifications-Session')
const timerVault=createAnnouncementVault('timer','X-Super-Notifications-Session')
const data=ref(null),error=ref(''),feedback=ref(''),busy=ref(false),loading=ref(true),title=ref(''),body=ref(''),now=ref(Date.now()),offset=ref(0)
const installURL='https://imperiobarbaro.com.br/instalar'
let generation=0,timer,poll
const timestamp=computed(()=>Math.floor((now.value+offset.value)/1000))
function remaining(kind){return Math.max(0,(data.value?.cooldowns?.[kind]||0)-timestamp.value)}
function waitLabel(kind){const n=remaining(kind);return n?`Disponível em ${Math.floor(n/60)}:${String(n%60).padStart(2,'0')}`:'Enviar aviso'}
function warDate(value){return Date.parse(value?.replace(/^(\d{4})(\d{2})(\d{2})T(\d{2})(\d{2})(\d{2})\.000Z$/,'$1-$2-$3T$4:$5:$6Z'))}
function warBlock(kind){
 if(!kind.startsWith('war-'))return ''
 if(props.warLoading)return 'Conferindo guerra…'
 const start=warDate(props.war?.startTime),end=warDate(props.war?.endTime)
 if(props.war?.state!=='inWar'||props.war?.attacksPerMember!==2||!Number.isFinite(start)||!Number.isFinite(end)||now.value+offset.value<start||now.value+offset.value>=end)return 'Aguarde uma guerra com 2 ataques em andamento.'
 if(kind==='war-second'&&now.value+offset.value<start+12*3600000)return 'Liberado após 12 horas de guerra.'
 return ''
}
async function load(silent=false){
 const current=++generation,clan=props.clanId
 if(!silent){loading.value=true;data.value=null;error.value=''}
 try{const value=await adminAPI('/api/announcements?clan='+clan);if(current!==generation||clan!==props.clanId)return;data.value=value;offset.value=value.serverTime*1000-Date.now()}
 catch(e){if(current===generation)error.value=e.message}
 finally{if(current===generation)loading.value=false}
}
async function send(kind){
 if(busy.value||!data.value||remaining(kind)||warBlock(kind))return
 const clan=props.clanId
 busy.value=true;error.value='';feedback.value=''
 try{
  const input={clanId:clan,kind,...(kind==='custom'?{title:title.value,body:body.value}:{})}
  const result=kind==='custom'?await vault.request('/api/announcements/custom',{method:'POST',body:JSON.stringify(input)}):await adminAPI('/api/announcements',{method:'POST',body:JSON.stringify(input)})
  if(clan!==props.clanId)return
  feedback.value=`Aviso na fila para ${result.queued} celular(es). Acompanhe o envio abaixo.`
  data.value.cooldowns[kind]=result.nextAt
  if(kind==='custom'){title.value='';body.value=''}
 }catch(e){if(clan===props.clanId){error.value=e.message;if([401,423].includes(e.status)&&kind==='custom')vault.lock(e.message)}}
 finally{busy.value=false;await load(true)}
}
function date(n){return new Date(n*1000).toLocaleString('pt-BR')}
async function copy(){try{await navigator.clipboard.writeText(installURL);feedback.value='Link de instalação copiado.'}catch{feedback.value='Copie o link de instalação exibido acima.'}}
async function resetTimer(kind){
 if(busy.value||!timerVault.unlocked.value)return
 const clan=props.clanId
 busy.value=true;error.value='';feedback.value=''
 try{await timerVault.request('/api/announcements/timer/reset',{method:'POST',body:JSON.stringify({clanId:clan,kind})});if(clan===props.clanId)feedback.value='Intervalo liberado para este botão. O próximo envio inicia uma nova hora.'}
 catch(e){if(clan===props.clanId)error.value=e.message}
 finally{busy.value=false;await load(true)}
}
function lockAccess(){if(document.hidden){vault.lock();timerVault.lock()}}
function lockAll(){vault.lock();timerVault.lock()}
watch(()=>props.clanId,()=>{feedback.value='';load()})
onMounted(()=>{document.addEventListener('visibilitychange',lockAccess);window.addEventListener('pagehide',lockAll);load();timer=setInterval(()=>{now.value=Date.now()},1000);poll=setInterval(()=>{if(!document.hidden&&!busy.value)load(true)},15000)})
onUnmounted(()=>{document.removeEventListener('visibilitychange',lockAccess);window.removeEventListener('pagehide',lockAll);lockAll();generation++;clearInterval(timer);clearInterval(poll)})
</script>
<template>
 <section class="announcements-panel">
  <div class="announcements-intro"><div><small>O IMPÉRIO NO CELULAR</small><h2>Um toque. Um lembrete.</h2><p>Todos os celulares inscritos no clã selecionado recebem o aviso. As mensagens de guerra indicam a quem o lembrete se aplica. Para receber neste celular, abra o app de membros, escolha este mesmo clã e ative os avisos.</p></div><div class="install-share"><label>Link para os novos membros<input :value="installURL" readonly aria-label="Link de instalação"></label><button class="secondary" @click="copy">Copiar link</button><a :href="installURL" target="_blank" rel="noopener">Preparar este celular ↗</a></div></div>
  <p v-if="loading" role="status">Carregando comunicados…</p>
  <p v-if="error" class="notice error" role="alert">{{ error }} <button :disabled="busy" @click="load()">Atualizar</button></p>
  <p v-if="feedback" class="notice" role="status">{{ feedback }}</p>
  <template v-if="data">
   <div class="announcement-reach"><strong>{{ data.subscribers }}</strong><span>celular(es) inscrito(s) neste clã</span><span>Intervalo de <b>1 hora por botão</b>, compartilhado entre os líderes.</span></div>
   <p v-if="!data.subscribers" class="notice">Compartilhe o link de instalação. O envio fica disponível quando houver um celular inscrito neste clã.</p>
   <section class="timer-access custom-announcement"><div class="section"><div><small>ACESSO PROTEGIDO</small><h2>Liberar intervalo</h2><p>Use a senha de liberação para antecipar um botão. Isso não envia um aviso e mantém a regra das 12 horas de guerra.</p></div><button v-if="timerVault.unlocked.value" class="secondary" @click="timerVault.lock()">Bloquear liberação</button></div>
    <p v-if="!data.timerConfigured" class="notice">A senha de liberação do intervalo ainda não foi configurada.</p>
    <form v-else-if="!timerVault.unlocked.value" @submit.prevent="timerVault.unlock"><label>Senha de liberação do intervalo<input v-model="timerVault.password.value" type="password" autocomplete="off" required :disabled="timerVault.busy.value"></label><button class="secondary" :disabled="timerVault.busy.value">{{ timerVault.busy.value?'Desbloqueando…':'Desbloquear liberação' }}</button></form>
    <p v-else class="notice">Liberação disponível por dez minutos. Use “Liberar agora” no botão desejado. Ao ocultar o painel, este acesso bloqueia.</p><p v-if="timerVault.error.value" role="alert">{{ timerVault.error.value }}</p>
   </section>
   <div class="announcement-grid"><article v-for="item in data.templates" :key="item.kind" class="announcement-card"><small>{{ item.label }}</small><h3>{{ item.title }}</h3><p>{{ item.body }}</p><button class="primary" :disabled="busy||!data.subscribers||remaining(item.kind)>0||!!warBlock(item.kind)" @click="send(item.kind)">{{ remaining(item.kind)?waitLabel(item.kind):warBlock(item.kind)||'Enviar aviso' }}</button><button v-if="remaining(item.kind)>0&&timerVault.unlocked.value" class="secondary" :disabled="busy" @click="resetTimer(item.kind)">Liberar agora</button></article></div>
   <section class="custom-announcement"><div class="section"><div><small>ACESSO PROTEGIDO</small><h2>Aviso personalizado</h2><p>Use a senha exclusiva de avisos personalizados. A sessão bloqueia ao ocultar o painel e expira em dez minutos.</p></div><button v-if="vault.unlocked.value" class="secondary" @click="vault.lock()">Bloquear</button></div>
    <p v-if="!data.customConfigured" class="notice">A senha de avisos personalizados ainda não foi configurada.</p>
    <form v-else-if="!vault.unlocked.value" @submit.prevent="vault.unlock"><label>Senha de avisos personalizados<input v-model="vault.password.value" type="password" autocomplete="off" required :disabled="vault.busy.value"></label><button class="primary" :disabled="vault.busy.value">{{ vault.busy.value?'Desbloqueando…':'Desbloquear' }}</button><p v-if="vault.error.value" role="alert">{{ vault.error.value }}</p></form>
    <form v-else @submit.prevent="send('custom')"><label>Título<input v-model="title" maxlength="70" required placeholder="Ex.: Reunião da liderança"></label><label>Mensagem curta<textarea v-model="body" maxlength="240" required rows="3" placeholder="Escreva o que o membro precisa saber ou fazer."></textarea><small>{{ body.length }}/240</small></label><div v-if="title||body" class="announcement-preview"><small>PRÉVIA NO CELULAR · IMPÉRIO BÁRBARO</small><strong>{{ title||'Título do aviso' }}</strong><p>{{ body||'Mensagem do aviso' }}</p></div><button class="primary" :disabled="busy||!data.subscribers||remaining('custom')>0||!title.trim()||!body.trim()">{{ waitLabel('custom') }}</button><button v-if="remaining('custom')>0&&timerVault.unlocked.value" type="button" class="secondary" :disabled="busy" @click="resetTimer('custom')">Liberar agora</button></form>
   </section>
   <section class="announcement-history"><div class="section"><h2>Últimos 10 comunicados</h2><button class="secondary" :disabled="busy" @click="load(true)">Atualizar envios</button></div><p class="report-note">Aceito pelo serviço de push não confirma leitura. Som, destaque e entrega dependem das configurações do celular. Avisos pendentes expiram em uma hora. Os 10 comunicados mais recentes ficam no histórico; os anteriores são apagados após concluir os envios.</p><p v-if="!data.items.length">Nenhum comunicado enviado neste clã.</p><article v-for="item in data.items" :key="item.id"><small>{{ date(item.createdAt) }}</small><h3>{{ item.title }}</h3><p>{{ item.body }}</p><p class="report-note">{{ item.accepted }} aceito(s) pelo serviço · {{ item.pending }} na fila · {{ item.failed }} sem entrega · {{ item.recipients }} destinatário(s)</p></article></section>
  </template>
 </section>
</template>
<style scoped>
.announcements-intro{display:flex;justify-content:space-between;gap:32px;padding:24px 0}.announcements-intro h2{font-size:28px;margin:10px 0}.announcements-intro p{max-width:540px;line-height:1.7;color:#656a5c}.install-share{min-width:260px;align-self:center}.install-share input{width:100%;margin:8px 0}.announcement-reach{display:flex;align-items:center;gap:12px;padding:18px 22px;background:#e4e6d8;margin:18px 0 24px;flex-wrap:wrap}.announcement-reach>strong{font-size:30px}.announcement-reach>span:last-child{margin-left:auto;font-size:12px}.timer-access{margin-bottom:24px}.install-share a{display:block;margin-top:14px;color:#655127}.announcement-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(260px,1fr));gap:18px}.announcement-card{display:flex;flex-direction:column;padding:26px;border:1px solid #cecbb9;border-top:3px solid #a57d29;background:#f8f6ed}.announcement-card small{color:#7c6533}.announcement-card h3{line-height:1.5;margin:16px 0 10px;font-size:19px}.announcement-card p{line-height:1.7;flex:1;color:#5a614f}.announcement-card button{margin-top:18px;min-height:46px;white-space:normal}.custom-announcement{padding:28px;border:1px solid #c9c4b1;margin-top:32px}.custom-announcement form{display:grid;gap:18px;max-width:600px}.custom-announcement label{display:grid;gap:8px}.custom-announcement input,.custom-announcement textarea{width:100%;padding:12px;font:inherit}.custom-announcement form>button{justify-self:start;min-height:44px}.announcement-preview{padding:20px;background:#22291f;color:#f7f2de;border-left:4px solid #d9ac4a;overflow-wrap:anywhere}.announcement-preview strong{display:block;margin-top:12px}.announcement-preview p{line-height:1.6;white-space:pre-line}.announcement-history{margin-top:32px}.announcement-history article{padding:18px 0;border-bottom:1px solid #d1cfbf}.announcement-history h3{margin:10px 0}.announcement-history p{line-height:1.7;white-space:pre-line;overflow-wrap:anywhere}button:disabled{opacity:.55;cursor:not-allowed}@media(max-width:700px){.announcements-intro{display:block}.install-share{margin-top:24px}.announcement-reach>span:last-child{margin-left:0}.custom-announcement{padding:20px}}
</style>
