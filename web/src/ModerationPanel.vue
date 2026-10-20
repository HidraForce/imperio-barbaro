<script setup>
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { adminAPI } from './admin-api.js'
import { clanPresence } from './member-presence.js'
import { moderationSummary, moderationLabel, moderationSectorCounts, moderationStatus, moderationSector, moderationSectors, moderationSanctions, moderationMessage, punishmentListMessage } from './moderation.js'
import ModerationRules from './ModerationRules.vue'
import WhatsAppMessage from './WhatsAppMessage.vue'
import { moderationStatuses as statuses, moderationCSV, downloadCSV } from './admin-reports.js'
const props = defineProps({ clanId: String, clanName: String, members: Array, phones:{type:Object,default:()=>({})} })
const emit = defineEmits(['changed'])
const records = ref([]), settings = ref(null), busy = ref(false), loaded = ref(false), error = ref(''), feedback = ref(''), filter = ref('all'), editing = ref(null), removal = ref(null), copyText = ref('')
const empty = () => ({ playerTag: '', playerName: '', kind: 'warning', sector: '', sanction: '', status: 'active', reason: '', action: '', notes: '' })
const form = ref(empty()), selectedMember = ref('')
const now = ref(Date.now())
let expiryTimer
const summary = computed(() => moderationSummary(records.value, now.value))
const selectedSummary = computed(() => summary.value.find(player => player.tag.replace(/^#/, '').toUpperCase() === form.value.playerTag.replace(/^#/, '').trim().toUpperCase()))
const editableStatuses = computed(() => Object.entries(statuses).filter(([key]) => key !== 'expired'))
const statusOf = record => moderationStatus(record, now.value)
watch(() => form.value.sanction, value => { if (moderationSanctions[value]) form.value.action = moderationSanctions[value] })
const decisions = computed(() => summary.value.filter(player => player.needsDecision))
const messagePlayer=ref('')
const punishedPlayers=computed(()=>[...new Map(records.value.filter(record=>record.kind==='punishment'&&record.status==='active'&&record.action?.trim()).map(record=>[record.playerTag,{tag:record.playerTag,name:record.playerName}])).values()])
const punishmentMessage=computed(()=>punishmentListMessage(records.value,props.clanName,messagePlayer.value,props.phones,props.members))
watch(()=>[props.phones,props.members],()=>{copyText.value='';feedback.value='';})
watch(punishedPlayers,players=>{if(messagePlayer.value&&!players.some(player=>player.tag===messagePlayer.value))messagePlayer.value=''})
const visible = computed(() => records.value.filter(record => {
  if (filter.value === 'dismissed') return record.status === 'dismissed'
  if (record.status === 'dismissed') return false
  if (['pending', 'expired'].includes(filter.value)) return statusOf(record) === filter.value
  return filter.value === 'all' || record.kind === filter.value
}))
const memberPresence=tag=>clanPresence(tag,props.members)
const url = path => path + '?clan=' + props.clanId
async function load() {
  const data = await adminAPI(url('/api/moderation'))
  now.value = Date.now(); records.value = data.items; settings.value = data.settings; loaded.value = true
  emit('changed', {clanId:props.clanId, items:data.items})
}
async function perform(action) {
  if (busy.value) return
  busy.value = true; error.value = ''; feedback.value = ''; copyText.value = ''
  try { await action() } catch (failure) { error.value = failure.message }
  finally { busy.value = false }
}
function choosePlayer(event) { const member = props.members?.find(m => m.tag === event.target.value); if (member) { form.value.playerTag = member.tag; form.value.playerName = member.name } }
function edit(record) { editing.value = record.id; form.value = { ...record, sector: moderationSector(record) }; feedback.value = '' }
function cancel() { editing.value = null; form.value = empty(); selectedMember.value = '' }
async function save() {
  await perform(async () => {
    const payload = Object.fromEntries(['revision', 'playerTag', 'playerName', 'kind', 'sector', 'sanction', 'status', 'reason', 'action', 'notes'].filter(key => form.value[key] !== undefined).map(key => [key, form.value[key]]))
    await adminAPI(url('/api/moderation' + (editing.value ? '/' + editing.value : '')), { method: editing.value ? 'PUT' : 'POST', body: JSON.stringify(payload) })
    cancel(); await load(); feedback.value = 'Registro salvo no histórico deste clã.'
  })
}
function suggest(player) { cancel(); form.value = { ...empty(), playerTag: player.tag, playerName: player.name, kind: 'punishment', reason: `${player.warnings} avisos ativos. ${moderationSectorCounts(player)}.`, sanction: player.degree === 4 ? 'expulsion' : '', action: player.degree === 4 ? moderationSanctions.expulsion : '' } }
async function remove(record) {
  await perform(async () => {
    const payload = Object.fromEntries(['revision', 'playerTag', 'playerName', 'kind', 'sector', 'sanction', 'reason', 'action', 'notes'].map(key => [key, record[key]])); payload.status = 'dismissed'
    await adminAPI(url('/api/moderation/' + record.id), { method: 'PUT', body: JSON.stringify(payload) })
    removal.value = null; if (editing.value === record.id) cancel(); await load(); feedback.value = 'Removido das listas. A sincronização não criará esta ocorrência novamente.'
  })
}
async function toggleAutomatic(event) {
  const automatic = event.target.checked
  await perform(async () => { await adminAPI(url('/api/moderation-settings'), { method: 'PUT', body: JSON.stringify({ revision: settings.value.revision, automatic }) }); await load(); feedback.value = automatic ? 'Consulta automática ativada a cada 15 minutos, mesmo com o painel fechado.' : 'Consulta automática desativada; o histórico foi preservado.' })
  event.target.checked = settings.value.automatic
}
async function sync() {
  await perform(async () => { const result = await adminAPI(url('/api/moderation/sync'), { method: 'POST', body: '{}' }); await load(); feedback.value = `${result.created} novo(s) aviso(s). Ocorrências existentes e removidas não são duplicadas.` })
}
async function copy(record) {
  const phones=props.phones,members=props.members
  const text = moderationMessage(record, props.clanName, props.phones,memberPresence(record.playerTag))
  try { await navigator.clipboard.writeText(text); if(phones===props.phones&&members===props.members)feedback.value = 'Mensagem copiada. Cole no WhatsApp.' } catch { if(phones===props.phones&&members===props.members){copyText.value = text; feedback.value = 'Copie a mensagem abaixo.'} }
}
onMounted(() => { perform(load); expiryTimer = setInterval(() => { now.value = Date.now() }, 1000) })
onUnmounted(() => clearInterval(expiryTimer))
</script>
<template>
  <section class="admin-editor">
    <div class="section"><div><small>COOPERAÇÃO E DISCIPLINA · SOMENTE ESTE CLÃ</small><h2>Avisos e punições</h2><p>Graus calculados pelos avisos ativos de cada membro. A liderança registra a medida aplicada.</p></div></div>
    <ModerationRules/>
    <p v-if="error" class="notice error" role="alert">{{ error }}</p><p v-if="feedback" class="notice" role="status">{{ feedback }}</p>
    <p v-if="busy" role="status">Consultando ou salvando…</p>
    <button v-if="!loaded && !busy" class="secondary" @click="perform(load)">Tentar novamente</button>
    <template v-if="loaded">
      <fieldset :disabled="busy"><legend>Avisos automáticos</legend>
        <label class="check-label"><input type="checkbox" :checked="settings.automatic" @change="toggleAutomatic"> Consultar automaticamente a cada 15 minutos</label>
        <p class="report-note">Guerra/CWL: somente jogadores escalados com zero ataques em atividades encerradas. Raids: zero ataques exige conferir elegibilidade e participação em outro clã antes de contar como aviso. Dados ausentes e ataques parciais não geram avisos automáticos. A primeira consulta considera os últimos sete dias disponíveis; guerras antigas que já saíram da API não podem ser recuperadas.</p>
        <p class="report-note">A lista de raids precisa ter sido capturada durante o evento para identificar quem não participou. Ative a automação antes do próximo fim de semana para registrar essa lista. Entradas/retornos são acompanhados a cada 15 minutos: quem comprovadamente entrou depois do evento é excluído; entrada durante o evento ou data incerta fica a conferir e não conta como aviso ativo. O primeiro registro é uma referência, não a data real de entrada. Nenhuma mensagem ou expulsão é executada automaticamente.</p>
        <p v-if="settings.lastCheck" class="report-note">Última consulta: {{ new Date(settings.lastCheck).toLocaleString('pt-BR') }}</p>
        <p v-for="problem in settings.problems" :key="problem" class="notice error">{{ problem }}</p>
        <div class="editor-actions"><button class="secondary" @click="sync">Verificar zero ataques agora</button><button class="secondary" @click="perform(load)">Atualizar histórico</button></div>
      </fieldset>
      <section v-if="summary.length" class="discipline-decisions"><h3>Situação dos membros</h3><p v-if="decisions.length">{{ decisions.length }} membro(s) aguardando decisão da liderança.</p>
        <article v-for="player in summary" :key="player.tag" class="discipline-player">
          <div class="editor-actions"><strong>{{ player.name }} · {{ player.tag }}</strong><span class="badge" :class="{ 'degree-expulsion': player.degree === 4 }">{{ moderationLabel(player) }}</span><span v-if="memberPresence(player.tag)===false" class="former-member-badge">Fora do clã atual</span></div>
          <p>{{ player.warnings }} aviso(s) ativo(s) · {{ moderationSectorCounts(player) }}</p>
          <p v-if="player.pending" class="report-note">{{ player.pending }} ocorrência(s) a conferir ou com setor a definir.</p>
          <p v-if="player.expiresAt" class="report-note">Expiram em {{ new Date(player.expiresAt).toLocaleString('pt-BR') }} se não houver novo aviso.</p>
          <button v-if="player.needsDecision" class="secondary" :disabled="busy" @click="suggest(player)">{{ player.degree === 4 ? 'Registrar expulsão' : 'Registrar punição' }}</button>
        </article>
      </section>
      <form @submit.prevent="save"><fieldset :disabled="busy"><legend>{{ editing ? 'Editar registro' : 'Adicionar aviso ou punição manualmente' }}</legend>
        <label v-if="!editing">Selecionar membro <select v-model="selectedMember" @change="choosePlayer"><option value="">Preencher manualmente</option><option v-for="member in members" :key="member.tag" :value="member.tag">{{ member.name }} · {{ member.tag }}</option></select></label>
        <div class="editor-grid"><label>Jogador<input v-model="form.playerName" maxlength="100" required :readonly="form.source && form.source !== 'manual'"></label><label>Tag<input v-model="form.playerTag" maxlength="16" required placeholder="#P0Y8LQ" :readonly="form.source && form.source !== 'manual'"></label></div>
        <div class="editor-grid"><label>Lista<select v-model="form.kind" :disabled="!!form.source && form.source !== 'manual'"><option value="warning">Avisos</option><option value="punishment">Punições</option></select></label><label>Situação<select v-model="form.status"><option v-for="[status, label] in editableStatuses" :key="status" :value="status">{{ label }}</option></select></label></div>
        <label v-if="form.kind === 'warning'">Setor do aviso<select v-model="form.sector" :required="['active', 'pending'].includes(form.status)" :disabled="!!form.source && form.source !== 'manual'"><option value="">Selecione o setor</option><option v-for="(label, sector) in moderationSectors" :key="sector" :value="sector">{{ label }}</option></select></label>
        <p v-if="selectedSummary" class="report-note">Situação atual: {{ moderationLabel(selectedSummary) }} · {{ moderationSectorCounts(selectedSummary) }}. O grau será recalculado ao salvar.</p>
        <label>Motivo<textarea v-model="form.reason" maxlength="1500" rows="3" required :readonly="form.source && form.source !== 'manual'"></textarea></label>
        <label v-if="form.kind === 'punishment'">Medida definida pela liderança<select v-model="form.sanction" :required="!editing || !!form.sanction"><option value="">{{ editing && !form.sanction ? 'Medida histórica: ' + form.action : 'Selecione a medida' }}</option><option v-for="(label, sanction) in moderationSanctions" :key="sanction" :value="sanction">{{ label }}</option></select></label>
        <label>Justificativa e observações<textarea v-model="form.notes" maxlength="2000" rows="3"></textarea></label>
        <p v-if="form.status === 'pending'" class="report-note">Este registro ainda não conta como aviso. Após conferir, mude a situação para Ativo; se houver justificativa, selecione Justificado.</p>
        <div class="editor-actions"><button class="primary">{{ editing ? 'Salvar registro' : 'Adicionar à lista' }}</button><button type="button" class="secondary" @click="cancel">{{ editing ? 'Cancelar edição' : 'Limpar formulário' }}</button></div>
      </fieldset></form>
      <div class="section"><h2>Histórico</h2><div class="editor-actions"><label>Exibir<select v-model="filter"><option value="all">Avisos e punições</option><option value="warning">Avisos</option><option value="punishment">Punições</option><option value="pending">A conferir</option><option value="expired">Expirados</option><option value="dismissed">Removidos</option></select></label><button class="secondary" :disabled="busy || !visible.length" @click="downloadCSV(moderationCSV(visible, clanName), 'avisos-punicoes-cla-'+clanId+'-'+filter+'.csv')">↓ Exportar CSV (filtro)</button></div></div>
      <WhatsAppMessage title="Compartilhar lista de punições" :message="punishmentMessage" :busy="busy" empty="Nenhuma punição ativa com medida definida para compartilhar.">
        <p class="report-note">Inclui somente punições ativas deste clã, com motivo e medida definidos pela liderança. Registros a conferir, justificados, encerrados ou removidos e observações internas ficam fora da mensagem.</p>
        <label class="message-recipient">Destinatários da mensagem de punições<select v-model="messagePlayer" :disabled="busy"><option value="">Lista completa de punições ativas</option><option v-for="player in punishedPlayers" :key="player.tag" :value="player.tag">{{ player.name }} · {{ player.tag }}{{ memberPresence(player.tag)===false?' · Fora do clã atual':'' }}</option></select></label>
      </WhatsAppMessage>
      <article v-for="record in visible" :key="record.id" class="moderation-record">
        <div class="editor-actions"><h3>{{ record.playerName }} <small>{{ record.playerTag }}</small></h3><span v-if="memberPresence(record.playerTag)===false" class="former-member-badge">Fora do clã atual · histórico</span><span class="badge">{{ record.kind === 'punishment' ? 'Punição' : 'Aviso' }} · {{ statuses[statusOf(record)] }}</span></div>
        <p v-if="record.kind === 'warning'" class="report-note">Setor: {{ moderationSectors[moderationSector(record)] || 'A definir · não conta para os graus' }}<span v-if="record.issuedAt"> · Confirmado em {{ new Date(record.issuedAt).toLocaleString('pt-BR') }}</span><span v-if="record.expiresAt"> · {{ statusOf(record) === 'expired' ? 'Expirou em' : 'Validade até' }} {{ new Date(record.expiresAt).toLocaleString('pt-BR') }}</span></p>
        <p>{{ record.reason }}</p><p v-if="record.action"><strong>Medida:</strong> {{ record.action }}</p><p v-if="record.notes"><strong>Observações:</strong> {{ record.notes }}</p>
        <p class="report-note">{{ record.source === 'manual' ? 'Registro manual' : 'Registro automático · ' + record.source }} · {{ new Date(record.createdAt).toLocaleString('pt-BR') }}</p>
        <div class="editor-actions"><button class="secondary" :disabled="busy" @click="edit(record)">Editar / justificar</button><button class="secondary" :disabled="busy" @click="copy(record)">Copiar mensagem</button><button v-if="record.status !== 'dismissed'" class="secondary" :disabled="busy" @click="removal = record.id">Remover da lista</button></div>
        <div v-if="removal === record.id" class="notice"><p>Remover este registro das listas? Ele deixará de contar para punições e não será recriado na sincronização.</p><div class="editor-actions"><button class="secondary" :disabled="busy" @click="remove(record)">Confirmar remoção</button><button class="secondary" @click="removal = null">Cancelar</button></div></div>
      </article>
      <p v-if="!visible.length" class="empty">Nenhum registro neste filtro.</p>
      <label v-if="copyText">Mensagem para copiar<textarea :value="copyText" readonly rows="8" @focus="$event.target.select()"></textarea></label>
    </template>
  </section>
</template>
