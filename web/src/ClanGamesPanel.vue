<script setup>
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { adminAPI } from './admin-api.js'
import { downloadCSV, roleLabel } from './admin-reports.js'
import { buildObservedClanGamesProgress, clanGamesCSV, clanGamesPoints, clanGamesStatus, visibleClanGames } from './clan-games.js'

const props = defineProps({ clanId: String, configured: Boolean })
const selectedSeason = ref(new Date().toISOString().slice(0, 7))
const data = ref(null), loading = ref(false), busy = ref(false), error = ref(''), feedback = ref(''), stale = ref(false)
const statusFilter = ref('all'), presenceFilter = ref('all'), settingsOpen = ref(false), form = ref(null)
let generation = 0, poll
const seasons = computed(() => [...new Set([selectedSeason.value, ...(data.value?.seasons || [])])].sort().reverse())
const event = computed(() => data.value?.event)
const summary = computed(() => data.value?.summary || {})
const observed = computed(() => buildObservedClanGamesProgress(data.value?.items || [], event.value))
const rows = computed(() => visibleClanGames(data.value?.items || [], statusFilter.value, presenceFilter.value))
const stateLabel = computed(() => event.value?.state === 'ended' ? event.value.finalized ? 'Encerrado · coleta concluída' : 'Encerrado · aguardando conferência' : ({ upcoming: 'Antes do início', active: 'Em andamento' })[event.value?.state] || 'Situação indisponível')
const progress = computed(() => event.value?.clanTarget > 0 ? Math.min(100, Math.max(0, (summary.value.points || 0) / event.value.clanTarget * 100)) : 0)
const targetFields = [
  ['memberTarget', 'Meta de Membro'], ['elderTarget', 'Meta de Ancião'],
  ['coLeaderTarget', 'Meta de Colíder'], ['leaderTarget', 'Meta de Líder'],
  ['clanTarget', 'Meta total do clã'], ['playerCap', 'Limite individual desta edição'],
]
function number(value) { return Number.isFinite(value) ? value.toLocaleString('pt-BR') : '—' }
function date(value) {
  const instant = new Date(value)
  return value && Number.isFinite(instant.getTime()) ? instant.toLocaleString('pt-BR', { dateStyle: 'short', timeStyle: 'short' }) : 'Sem consulta'
}
function seasonLabel(value) {
  if (!/^\d{4}-\d{2}$/.test(value)) return value
  return new Date(value + '-01T12:00:00Z').toLocaleDateString('pt-BR', { month: 'long', year: 'numeric', timeZone: 'UTC' })
}
function localDateTime(value) {
  const instant = new Date(value)
  if (!Number.isFinite(instant.getTime())) return ''
  const parts = [instant.getFullYear(), String(instant.getMonth() + 1).padStart(2, '0'), String(instant.getDate()).padStart(2, '0')]
  return parts.join('-') + 'T' + String(instant.getHours()).padStart(2, '0') + ':' + String(instant.getMinutes()).padStart(2, '0')
}
function editSettings() {
  if (!event.value) return
  form.value = {
    ...Object.fromEntries(['season', 'revision', ...targetFields.map(([key]) => key)].map(key => [key, event.value[key]])),
    startTime: localDateTime(event.value.startTime), endTime: localDateTime(event.value.endTime),
  }
}
function toggleSettings(toggle) {
  settingsOpen.value = toggle.target.open
  if (settingsOpen.value && !form.value) editSettings()
}
function url(path, clan, season) {
  return path + '?' + new URLSearchParams({ clan, season }).toString()
}
async function request(path = '/api/clan-games', options, message = '') {
  if (!props.clanId || busy.value || loading.value || (path === '/api/clan-games/sync' && !props.configured)) return
  const current = ++generation, clan = props.clanId, season = selectedSeason.value
  const mutation = Boolean(options)
  if (mutation) busy.value = true
  else loading.value = true
  error.value = ''; feedback.value = ''
  try {
    const result = await adminAPI(url(path, clan, season), options)
    if (current !== generation || clan !== props.clanId || season !== selectedSeason.value) return
    data.value = result; stale.value = false; feedback.value = message
    if (!settingsOpen.value || path === '/api/clan-games/settings') editSettings()
  } catch (failure) {
    if (current !== generation || clan !== props.clanId || season !== selectedSeason.value) return
    error.value = failure.message; stale.value = Boolean(data.value)
  } finally {
    if (current === generation) { loading.value = false; busy.value = false }
  }
}
function sync() {
  return request('/api/clan-games/sync', { method: 'POST', body: '{}' }, 'Consulta concluída. Confira os horários e os registros a conferir.')
}
async function saveSettings() {
  const start = new Date(form.value.startTime), end = new Date(form.value.endTime)
  if (!Number.isFinite(start.getTime()) || !Number.isFinite(end.getTime()) || end <= start) {
    error.value = 'Informe um término posterior ao início dos jogos.'
    return
  }
  const payload = { ...form.value, startTime: start.toISOString(), endTime: end.toISOString() }
  for (const [key] of targetFields) payload[key] = Number(payload[key])
  await request('/api/clan-games/settings', { method: 'PUT', body: JSON.stringify(payload) }, 'Datas e metas salvas para esta edição.')
}
function exportRows() {
  if (event.value) downloadCSV(clanGamesCSV(rows.value, event.value, props.clanId), `jogos-cla-${props.clanId}-${event.value.season}.csv`)
}
watch(() => [props.clanId, props.configured, selectedSeason.value], () => {
  generation++; data.value = null; form.value = null; settingsOpen.value = false
  loading.value = false; busy.value = false; error.value = ''; feedback.value = ''; stale.value = false
  request()
}, { immediate: true })
onMounted(() => {
  poll = setInterval(() => {
    if (!document.hidden && !busy.value && !loading.value && !settingsOpen.value) request()
  }, 60000)
})
onUnmounted(() => { generation++; clearInterval(poll) })
</script>

<template>
  <section class="clan-games-panel" aria-labelledby="clan-games-title" :aria-busy="loading || busy">
    <div class="section games-heading">
      <div><small>PARTICIPAÇÃO · SOMENTE ESTE CLÃ</small><h2 id="clan-games-title">Quem está contribuindo</h2><p>Acompanhe os pontos observados e o progresso de cada membro.</p></div>
      <div class="games-actions">
        <label>Edição<select v-model="selectedSeason" :disabled="busy"><option v-for="season in seasons" :key="season" :value="season">{{ seasonLabel(season) }}</option></select></label>
        <button class="primary" :disabled="!configured || loading || busy" @click="sync">{{ busy ? 'Consultando ou salvando…' : 'Sincronizar jogos' }}</button>
      </div>
    </div>
    <p v-if="!configured" class="notice">Configure a API e a tag deste clã para novas coletas. O histórico salvo continua disponível.</p>
    <p v-if="loading && !data" class="loading" role="status">Carregando jogos do clã…</p>
    <p v-if="error" class="notice error" role="alert">{{ error }}</p>
    <p v-if="stale" class="notice" role="status">A atualização falhou. Exibindo os últimos registros salvos, que podem estar desatualizados.</p>
    <p v-if="feedback" class="notice" role="status">{{ feedback }}</p>
    <button v-if="clanId && !data && !loading && !busy" class="secondary" @click="request()">Tentar novamente</button>
    <template v-if="event">
      <div class="games-event">
        <div><span class="badge games-state" :class="event.state">{{ stateLabel }}</span><p>{{ date(event.startTime) }} a {{ date(event.endTime) }}</p></div>
        <span class="sync-note">Última coleta: {{ date(data.syncedAt) }}<span v-if="loading"> · Atualizando…</span></span>
      </div>
      <p v-if="event.state === 'ended' && !event.finalized" class="notice">O evento terminou e os registros ainda são provisórios. A coleta aguarda a atualização dos contadores após o encerramento.</p>
      <div v-if="observed.allPartial && event.state !== 'upcoming'" class="notice games-partial-notice" role="status">
        <strong>Acompanhamento parcial desta edição.</strong>
        <span v-if="observed.lateBaseline"> A primeira referência das contas acompanhadas foi registrada em {{ date(observed.firstBaselineAt) }}, com os jogos já iniciados. Os pontos feitos antes dessa coleta não podem ser recuperados.</span>
        <span v-else> As contas acompanhadas ainda não têm pontuação completa confirmada nesta edição.<span v-if="observed.firstBaselineAt"> Primeira referência disponível: {{ date(observed.firstBaselineAt) }}.</span></span>
        Os aumentos observados continuam aparecendo abaixo; os registros permanecem a conferir.
      </div>
      <div class="games-progress games-observed-progress">
        <div><span>Aumento observado nas contas acompanhadas</span><strong>{{ number(observed.points) }} <small>pontos observados</small></strong></div>
        <div class="games-observed-accounts"><strong>{{ number(observed.participants) }}</strong><span>conta(s) com aumento observado</span></div>
        <p>Soma somente as contas que estavam no clã na última coleta, desde a referência de cada uma. Pode estar incompleta ou incluir pontos feitos em outro clã; não é o total oficial do clã.<span v-if="observed.missing"> {{ number(observed.missing) }} conta(s) ainda sem pontuação disponível.</span></p>
      </div>
      <div class="games-progress">
        <div><span>Pontos confirmados desde referência anterior ao evento</span><strong>{{ number(summary.points) }} <small>/ {{ number(event.clanTarget) }}</small></strong></div>
        <progress :value="progress" max="100" aria-label="Progresso confirmado desde referência anterior ao evento">{{ Math.round(progress) }}%</progress>
        <p>Somente registros com referência válida antes do início e vínculo acompanhado entram nesta comparação com a meta. Os registros a conferir ficam fora.</p>
      </div>
      <div class="stats games-stats">
        <article><small>COM AUMENTO OBSERVADO</small><strong>{{ number(observed.participants) }}</strong><span>{{ number(summary.participants) }} com referência registrada antes do evento</span></article>
        <article><small>META CONFIRMADA</small><strong>{{ number(summary.completed) }}</strong><span>Atingiram a meta do cargo com referência válida</span></article>
        <article><small>ZERO CONFIRMADO</small><strong class="missing-count">{{ number(summary.zero) }}</strong><span>Com referência válida e vínculo acompanhado</span></article>
        <article><small>A CONFERIR</small><strong>{{ number(summary.review) }}</strong><span>Dados incompletos ou vínculo incerto</span></article>
      </div>
      <p class="report-note games-explanation">Os pontos vêm da diferença do contador “Games Champion”. A coleta automática acontece a cada 15 minutos, mesmo com o painel fechado. É preciso registrar a referência antes do início. Entradas, saídas e dados incompletos ficam a conferir; esta tela não gera avisos ou punições.</p>
      <div v-if="data.problems?.length" class="notice error" role="status"><strong>Coleta incompleta</strong><ul><li v-for="problem in data.problems" :key="problem">{{ problem }}</li></ul></div>
      <div class="games-filters">
        <label>Situação<select v-model="statusFilter"><option value="all">Todos os registros</option><option value="observed-positive">Com aumento observado</option><option value="observed-zero">Sem aumento observado desde a referência</option><option value="zero">Zero registrado (confirmado)</option><option value="below">Abaixo da meta (confirmado)</option><option value="contributing">Contribuindo (confirmado)</option><option value="completed">Meta cumprida</option><option value="review">A conferir</option><option value="ineligible">Não elegível</option></select></label>
        <label>Vínculo na coleta<select v-model="presenceFilter"><option value="all">Todos os vínculos</option><option value="current">No clã na última coleta</option><option value="former">Fora do clã na última coleta</option></select></label>
        <button class="secondary" :disabled="!rows.length" @click="exportRows">↓ Exportar CSV (filtro)</button>
      </div>
      <div class="table-wrap">
        <table class="games-table">
          <caption class="sr-only">Pontos e metas dos membros nos jogos do clã de {{ seasonLabel(event.season) }}</caption>
          <thead><tr><th scope="col">JOGADOR</th><th scope="col">PONTOS / META</th><th scope="col">SITUAÇÃO</th><th scope="col">ÚLTIMA CONSULTA</th></tr></thead>
          <tbody>
            <tr v-for="row in rows" :key="row.tag" :class="'games-row-' + row.status">
              <th scope="row"><b>{{ row.name || row.tag }}</b><small>{{ row.tag }} · {{ roleLabel(row.role) }}<span v-if="row.townHallLevel"> · CV {{ row.townHallLevel }}</span></small><span v-if="row.current === false" class="former-member-badge">Fora do clã na coleta</span></th>
              <td class="games-points"><b>{{ clanGamesPoints(row) }}</b><small>Meta: {{ number(row.target) }}</small><small v-if="row.partial">Parcial · a conferir</small></td>
              <td class="games-situation"><span class="games-status" :class="row.status"><i aria-hidden="true"></i>{{ clanGamesStatus(row) }}</span><small v-if="row.reason">{{ row.reason }}</small></td>
              <td class="games-updated">{{ date(row.updatedAt) }}<small v-if="row.baselineAt">Referência: {{ date(row.baselineAt) }}</small></td>
            </tr>
            <tr v-if="!rows.length"><td colspan="4" class="empty">{{ data.items?.length ? 'Nenhum registro neste filtro.' : 'Nenhum jogador registrado nesta edição. Sincronize para iniciar a coleta.' }}</td></tr>
          </tbody>
        </table>
      </div>
      <p class="report-note">“Sem aumento observado desde a referência” inclui contas com coleta parcial e não comprova falta de participação. “Zero registrado” exige referência válida anterior ao evento. O contador não identifica desafios iniciados e ainda não concluídos. Pontuações parciais ou a conferir pertencem ao jogador e podem ter sido obtidas em outro clã; ficam fora do total confirmado. Sem referência inicial, a pontuação completa desta edição não pode ser recuperada.</p>
      <details class="games-settings admin-editor" :open="settingsOpen" @toggle="toggleSettings">
        <summary>Datas e metas desta edição</summary>
        <form v-if="form" @submit.prevent="saveSettings">
          <p class="report-note">Confira no jogo o período e o limite individual desta edição. As metas por cargo e a meta do clã são definidas pela liderança. Datas no horário deste aparelho.</p>
          <fieldset :disabled="busy || loading"><legend>{{ seasonLabel(event.season) }}</legend>
            <div class="editor-grid"><label>Início<input v-model="form.startTime" type="datetime-local" required></label><label>Término<input v-model="form.endTime" type="datetime-local" required></label></div>
            <div class="editor-grid"><label v-for="[key, label] in targetFields" :key="key">{{ label }}<input v-model.number="form[key]" type="number" min="1" step="1" required></label></div>
            <div class="editor-actions"><button class="primary" type="submit">Salvar datas e metas</button><button class="secondary" type="button" @click="editSettings">Descartar edição</button></div>
          </fieldset>
        </form>
      </details>
    </template>
  </section>
</template>

<style scoped>
.games-heading{margin-top:0;align-items:center}.games-actions{display:flex;align-items:end;gap:12px;flex-wrap:wrap}.games-actions label,.games-filters label{display:grid;gap:7px;font-size:11px;color:var(--muted)}.games-event{display:flex;align-items:center;justify-content:space-between;gap:16px;flex-wrap:wrap;margin:24px 0 18px}.games-event p{font-size:12px;color:var(--muted);line-height:1.7;margin-bottom:0}.games-state{background:#e7e5dc;border-color:#b8b6aa;color:#66685c}.games-state.active{background:#e0e7ce;border-color:#a7b28a;color:#47612c}.games-event .sync-note{margin-top:0}.games-progress{display:grid;grid-template-columns:1fr minmax(140px,1fr);gap:12px 28px;align-items:center;padding:22px;border:1px solid #bca467;border-left:5px solid #b99137;background:#ede3bf;margin-bottom:20px}.games-progress span{font-size:12px;color:#6e613f}.games-progress strong{display:block;font-size:30px;color:#433921;margin-top:8px}.games-progress strong small{font-size:16px;font-weight:400;color:#6e613f}.games-progress progress{width:100%;height:13px;accent-color:#78602c}.games-progress p{font-size:11px;color:#6e613f;line-height:1.6;margin:0;grid-column:1/-1}.games-observed-progress{background:#e8ede9;border-color:#b7c2b6;border-left-color:#658060}.games-observed-accounts{text-align:right}.games-observed-accounts strong{margin:0 0 4px}.games-stats article:nth-child(2){border-top-color:#56854b}.games-stats article:nth-child(3){border-top-color:#b34d39}.games-stats article:nth-child(4){border-top-color:#8a8d80}.games-explanation{max-width:none}.games-filters{display:flex;align-items:end;flex-wrap:wrap;gap:16px;margin:24px 0 14px}.games-filters button{margin-left:auto}.games-table{min-width:760px}.games-table tbody th{background:transparent;border-bottom:1px solid #ddded2;height:auto;letter-spacing:0;min-width:170px;font-weight:400}.games-table tbody th b{display:block;font-size:13px;color:#30372a;overflow-wrap:anywhere}.games-table tbody th small{display:block;font-size:10px;font-weight:400;color:#74786b;margin-top:6px;line-height:1.5}.games-table .former-member-badge{margin-top:8px}.games-table .games-points b{font-size:15px;font-variant-numeric:tabular-nums}.games-table .games-points small{line-height:1.5}.games-situation{max-width:340px;min-width:230px}.games-situation>small{line-height:1.7;overflow-wrap:anywhere}.games-status{display:inline-flex;align-items:center;gap:8px;font-size:12px;line-height:1.5;color:#6e7267;white-space:nowrap}.games-status i{display:inline-block;flex-shrink:0;width:8px;height:8px;border-radius:50%;background:#8a8d80}.games-status.completed{color:#246540}.games-status.completed i{background:#2e8a4e}.games-status.contributing{color:#805b16}.games-status.contributing i{background:#b78821}.games-status.zero{color:#a22f29}.games-status.zero i{background:#cc4137}.games-updated{font-size:11px;min-width:145px;line-height:1.7}.games-row-zero th{border-left:3px solid #b03430}.games-settings{margin-top:28px;border:1px solid var(--line);padding:18px;background:var(--panel)}.games-settings>summary{font-size:13px;font-weight:700;cursor:pointer}.games-settings fieldset{margin-bottom:0}.games-settings .report-note{margin-bottom:24px}.games-settings .editor-grid label{min-width:0}.games-settings input{max-width:100%}.clan-games-panel button:disabled{opacity:.55;cursor:not-allowed}.notice ul{margin-bottom:0;padding-left:20px}.notice li{line-height:1.8}@media(max-width:1050px){.games-heading{align-items:flex-start;flex-wrap:wrap}.games-actions{width:100%;justify-content:space-between}}@media(max-width:700px){.games-observed-accounts{text-align:left}.games-actions{align-items:stretch}.games-actions label{flex:1;min-width:150px}.games-actions button{align-self:end}.games-progress{grid-template-columns:1fr;padding:18px}.games-progress strong{font-size:27px}.games-progress progress{margin:4px 0}.games-filters{gap:12px}.games-filters label{flex:1;min-width:140px}.games-filters button{margin-left:0;width:100%}.games-table th,.games-table td{padding:13px 12px}.games-settings{padding:14px}.games-stats strong{font-size:25px}}
</style>
