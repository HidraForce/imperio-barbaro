<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'

const clans = [
  { id: '1', name: 'Império Bárbaro', label: 'Clã principal' },
  { id: '2', name: 'Império Bárbar2', label: 'Clã secundário' },
]
const ruleLinks = [
  { name: 'Regras do grupo geral', url: 'https://imperiobarbaro.com.br/#regras-gerais' },
  { name: 'Regras do clã principal', url: 'https://imperiobarbaro.com.br/#regras-principal' },
  { name: 'Regras do clã secundário', url: 'https://imperiobarbaro.com.br/#regras-secundario' },
]
const items = ref([])
const revision = ref(null)
const snapshot = ref('[]')
const loading = ref(false)
const saving = ref(false)
const loaded = ref(false)
const error = ref('')
const feedback = ref('')
const copyFeedback = ref('')
const conflict = ref(false)
const confirmReload = ref(false)
const busy = computed(() => loading.value || saving.value)
const dirty = computed(() => loaded.value && JSON.stringify(items.value) !== snapshot.value)
const invalid = computed(() => items.value.some(item => !item.role.trim() || !item.description.trim()))
let requestController

function parseDocument(value) {
  if (!value || !Number.isSafeInteger(value.revision) || value.revision < 1 || !Array.isArray(value.items) || value.items.length > 40) {
    throw new Error('A resposta da liderança é inválida. Recarregue os dados salvos antes de editar.')
  }
  const ids = new Set()
  for (const item of value.items) {
    if (!item || typeof item.id !== 'string' || !item.id || ids.has(item.id) || !['1', '2'].includes(item.clanId) ||
      typeof item.role !== 'string' || item.role.length > 100 || typeof item.name !== 'string' || item.name.length > 100 ||
      typeof item.description !== 'string' || item.description.length > 600) {
      throw new Error('Os dados da liderança estão incompletos. Recarregue os dados salvos antes de editar.')
    }
    ids.add(item.id)
  }
  return { revision: value.revision, items: value.items.map(({ id, clanId, role, name, description }) => ({ id, clanId, role, name, description })) }
}

async function request(options = {}) {
  requestController = new AbortController()
  const response = await fetch('/api/leadership', {
    ...options,
    credentials: 'same-origin',
    cache: 'no-store',
    signal: requestController.signal,
    headers: { 'X-Clanboard-Request': '1', 'Content-Type': 'application/json' },
  })
  const result = await response.json().catch(() => null)
  if (!response.ok) {
    const failure = new Error(response.status === 401
      ? 'Sua sessão expirou. Entre novamente no painel para salvar a liderança.'
      : result?.error || result?.message || `Não foi possível acessar a liderança (${response.status}).`)
    failure.status = response.status
    throw failure
  }
  return parseDocument(result)
}

function acceptDocument(document) {
  items.value = document.items
  revision.value = document.revision
  snapshot.value = JSON.stringify(document.items)
  loaded.value = true
  conflict.value = false
  confirmReload.value = false
}

async function load() {
  if (busy.value) return
  loading.value = true
  error.value = ''
  feedback.value = ''
  try {
    acceptDocument(await request())
  } catch (failure) {
    if (failure.name !== 'AbortError') error.value = failure.message
  } finally {
    loading.value = false
  }
}

function requestReload() {
  if (busy.value) return
  if (dirty.value) confirmReload.value = true
  else load()
}

async function save() {
  if (busy.value || !loaded.value || !dirty.value || invalid.value || conflict.value) return
  saving.value = true
  error.value = ''
  feedback.value = ''
  try {
    const document = await request({
      method: 'PUT',
      body: JSON.stringify({
        revision: revision.value,
        items: items.value.map(item => ({ ...item, role: item.role.trim(), name: item.name.trim(), description: item.description.trim() })),
      }),
    })
    acceptDocument(document)
    feedback.value = 'Liderança salva. As alterações já estão disponíveis na página pública.'
  } catch (failure) {
    if (failure.name !== 'AbortError') {
      conflict.value = failure.status === 409
      error.value = conflict.value
        ? 'A liderança foi atualizada em outra sessão. Suas edições foram mantidas aqui. Copie o que precisar antes de recarregar os dados salvos e aplicar as mudanças novamente.'
        : failure.message
    }
  } finally {
    saving.value = false
  }
}

function addPosition(clanId) {
  if (busy.value || !loaded.value || items.value.length >= 40) return
  items.value.push({ id: crypto.randomUUID(), clanId, role: '', name: '', description: '' })
  feedback.value = ''
}

function removePosition(id) {
  if (busy.value) return
  items.value = items.value.filter(item => item.id !== id)
  feedback.value = ''
}

function movePosition(id, direction) {
  if (busy.value) return
  const index = items.value.findIndex(item => item.id === id)
  if (index === -1) return
  const groupIndices = items.value.flatMap((item, i) => item.clanId === items.value[index].clanId ? [i] : [])
  const next = groupIndices[groupIndices.indexOf(index) + direction]
  if (next === undefined) return
  ;[items.value[index], items.value[next]] = [items.value[next], items.value[index]]
  feedback.value = ''
}

async function copyLink(link) {
  try {
    await navigator.clipboard.writeText(link.url)
    copyFeedback.value = `Link copiado: ${link.name}.`
  } catch {
    copyFeedback.value = 'Não foi possível copiar automaticamente. Selecione e copie o endereço exibido abaixo do título.'
  }
}

function beforeLeaving(event) {
  if (!dirty.value) return
  event.preventDefault()
  event.returnValue = ''
}

onMounted(() => { load(); window.addEventListener('beforeunload', beforeLeaving) })
onBeforeUnmount(() => { requestController?.abort(); window.removeEventListener('beforeunload', beforeLeaving) })
</script>

<template>
  <section class="leadership-panel" aria-labelledby="leadership-heading">
    <div class="section leadership-heading">
      <div>
        <small>CONTEÚDO DA PÁGINA INICIAL</small>
        <h2 id="leadership-heading">Liderança pública</h2>
        <p>Atualize os responsáveis, cargos e funções dos dois clãs. As alterações aparecem para todos na página inicial após salvar.</p>
      </div>
      <a class="public-preview" href="https://imperiobarbaro.com.br/#lideranca" target="_blank" rel="noopener noreferrer">Ver página pública ↗</a>
    </div>

    <p v-if="loading" class="loading" role="status">Carregando liderança salva…</p>
    <p v-if="error" class="notice error" role="alert">{{ error }}</p>
    <p v-if="feedback && !dirty" class="leadership-success" role="status">{{ feedback }}</p>
    <button v-if="!loaded && !loading" type="button" class="secondary" @click="load">Tentar carregar novamente</button>

    <form v-if="loaded" class="leadership-editor" @submit.prevent="save" :aria-busy="busy">
      <div class="leadership-status">
        <span :class="{ unsaved: dirty }" role="status">{{ dirty ? 'Alterações ainda não salvas' : 'Exibindo a liderança salva' }}</span>
        <span>{{ items.length }} de 40 cargos · deixe o nome em branco para exibir “Vaga aberta”.</span>
      </div>

      <fieldset v-for="clan in clans" :key="clan.id" class="leadership-clan" :disabled="busy">
        <legend>{{ clan.label }} · {{ clan.name }}</legend>
        <p class="clan-help">A ordem abaixo será usada na página pública.</p>
        <article v-for="(item, index) in items.filter(position => position.clanId === clan.id)" :key="item.id" class="leadership-position">
          <div class="position-heading">
            <h3>{{ index + 1 }}. {{ item.role || 'Novo cargo' }} <span v-if="!item.name.trim()">Vaga aberta</span></h3>
            <div class="position-actions">
              <button type="button" class="secondary" :disabled="index === 0" :aria-label="'Mover cargo ' + (item.role || 'novo') + ' para cima'" @click="movePosition(item.id, -1)">↑</button>
              <button type="button" class="secondary" :disabled="index === items.filter(position => position.clanId === clan.id).length - 1" :aria-label="'Mover cargo ' + (item.role || 'novo') + ' para baixo'" @click="movePosition(item.id, 1)">↓</button>
              <button type="button" class="secondary remove-position" :aria-label="'Remover cargo ' + (item.role || 'novo')" @click="removePosition(item.id)">Remover</button>
            </div>
          </div>
          <div class="position-fields">
            <label :for="'role-' + item.id">Cargo<input :id="'role-' + item.id" v-model="item.role" type="text" maxlength="100" required placeholder="Ex.: Guerras · estratégia"></label>
            <label :for="'name-' + item.id">Nome do responsável <span>(opcional)</span><input :id="'name-' + item.id" v-model="item.name" type="text" maxlength="100" placeholder="Em branco = Vaga aberta"></label>
            <label :for="'description-' + item.id" class="position-description">Função e responsabilidades<textarea :id="'description-' + item.id" v-model="item.description" maxlength="600" required rows="3" placeholder="Explique quando os membros devem procurar esta pessoa."></textarea></label>
          </div>
        </article>
        <p v-if="!items.some(item => item.clanId === clan.id)" class="empty">Nenhum cargo neste clã. Adicione um cargo para exibi-lo na página inicial.</p>
        <button type="button" class="secondary" :disabled="items.length >= 40" @click="addPosition(clan.id)">＋ Adicionar cargo ao {{ clan.label.toLowerCase() }}</button>
      </fieldset>

      <div class="leadership-save-actions">
        <button class="primary" type="submit" :disabled="busy || !dirty || invalid || conflict">{{ saving ? 'Salvando liderança…' : 'Salvar liderança pública' }}</button>
        <button class="secondary" type="button" :disabled="busy" @click="requestReload">Recarregar dados salvos</button>
        <span v-if="invalid" class="field-hint">Preencha o cargo e a função de todas as posições.</span>
      </div>
      <div v-if="confirmReload" class="reload-confirmation" role="alert">
        <p>Recarregar descarta todas as alterações ainda não salvas. Deseja continuar?</p>
        <div>
          <button type="button" class="secondary" :disabled="busy" @click="load">Descartar alterações e recarregar</button>
          <button type="button" class="secondary" :disabled="busy" @click="confirmReload = false">Continuar editando</button>
        </div>
      </div>
    </form>

    <section class="leadership-rule-links" aria-labelledby="leadership-links-heading">
      <h3 id="leadership-links-heading">Links das regras para novos membros</h3>
      <p>Copie o link da seção que deseja compartilhar. Os membros podem consultar as regras sem entrar no painel.</p>
      <ul>
        <li v-for="link in ruleLinks" :key="link.url">
          <div><a :href="link.url" target="_blank" rel="noopener noreferrer">{{ link.name }} ↗</a><span>{{ link.url }}</span></div>
          <button type="button" class="secondary" :aria-label="'Copiar link: ' + link.name" @click="copyLink(link)">Copiar link</button>
        </li>
      </ul>
      <p class="copy-feedback" role="status">{{ copyFeedback }}</p>
    </section>
  </section>
</template>

<style scoped>
.leadership-heading{margin-top:0;align-items:flex-start}.leadership-heading h2{margin-bottom:10px}.leadership-heading p{max-width:720px}.public-preview{flex-shrink:0;font-size:12px;padding:8px 0}.leadership-status{display:flex;justify-content:space-between;gap:12px;flex-wrap:wrap;font-size:12px;color:var(--muted);padding:16px 0}.leadership-status .unsaved{font-weight:700;color:#805b16}.leadership-clan{margin:0 0 24px;padding:18px;border:1px solid var(--line);background:var(--panel);min-width:0}.leadership-clan legend{padding:0 8px;font-size:15px;font-weight:700;color:#3e5237}.clan-help{margin:0 0 14px;font-size:12px;color:var(--muted)}.leadership-position{padding:16px 0;margin-bottom:16px;border-bottom:1px solid var(--line)}.position-heading{display:flex;align-items:center;justify-content:space-between;gap:16px;margin-bottom:16px}.position-heading h3{margin:0;font-size:15px;overflow-wrap:anywhere}.position-heading h3 span{display:inline-block;font-size:10px;color:#805b16;background:#f5ecd2;margin:6px 0 0 8px;padding:5px 8px;vertical-align:middle}.position-actions{display:flex;gap:6px;flex-shrink:0}.position-actions .secondary{min-width:38px;padding:8px 11px}.remove-position{color:#963c30}.position-fields{display:grid;grid-template-columns:1fr 1fr;gap:16px}.position-fields label{font-size:12px;font-weight:700;color:#505746}.position-fields label>span{font-weight:400;color:var(--muted)}.position-fields input,.position-fields textarea{display:block;width:100%;min-height:42px;padding:10px;margin-top:8px;background:#fff;border:1px solid #a5a794;border-radius:2px;font:13px Arial,sans-serif;line-height:1.5;color:#282b24}.position-fields textarea{resize:vertical}.position-fields textarea:focus-visible{outline:3px solid #2777a3;outline-offset:3px}.position-description{grid-column:1/-1}.leadership-save-actions{display:flex;align-items:center;flex-wrap:wrap;gap:10px}.field-hint{font-size:12px;color:#805b16}.reload-confirmation{margin-top:16px;padding:16px;border:1px solid #c5aa66;background:#f5ecd2;font-size:12px;color:#654e18}.reload-confirmation p{margin:0 0 12px;line-height:1.6}.reload-confirmation>div{display:flex;gap:10px;flex-wrap:wrap}.leadership-success{padding:14px 16px;background:#e9f0df;border:1px solid #a9c3a2;font-size:12px;line-height:1.6;color:#356139}.leadership-rule-links{margin-top:32px;padding-top:24px;border-top:2px solid var(--line)}.leadership-rule-links h3{font-size:17px;margin:0 0 10px}.leadership-rule-links p{font-size:12px;color:var(--muted);line-height:1.6}.leadership-rule-links ul{list-style:none;padding:0;margin:16px 0 0;border:1px solid var(--line);background:var(--panel)}.leadership-rule-links li{display:flex;justify-content:space-between;align-items:center;gap:16px;padding:16px}.leadership-rule-links li+li{border-top:1px solid var(--line)}.leadership-rule-links li>div{min-width:0}.leadership-rule-links a{font-size:13px;font-weight:700}.leadership-rule-links li span{display:block;margin-top:7px;font-size:11px;color:var(--muted);overflow-wrap:anywhere;user-select:all}.leadership-rule-links li button{flex-shrink:0}.copy-feedback{min-height:20px}.leadership-clan:disabled{opacity:.75}
@media(max-width:700px){.leadership-heading{flex-direction:column;gap:10px}.leadership-clan{padding:14px}.leadership-clan legend{font-size:13px}.position-heading{align-items:flex-start;flex-direction:column;gap:10px}.position-fields{grid-template-columns:1fr}.leadership-rule-links li{align-items:flex-start;flex-direction:column;gap:12px}.leadership-save-actions>.primary{width:100%}.leadership-status{line-height:1.5}}
</style>
