<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { adminAPI } from './admin-api.js'
import RuleLines from './RuleLines.vue'
const draft = ref(null), saved = ref(''), busy = ref(false), error = ref(''), feedback = ref(''), conflict = ref(false), confirmReload = ref(false)
const dirty = computed(() => draft.value && JSON.stringify(draft.value) !== saved.value)
function accept(document) {
  draft.value = document
  saved.value = JSON.stringify(document)
  conflict.value = false
  confirmReload.value = false
}
async function load() {
  busy.value = true; error.value = ''; feedback.value = ''
  try { accept(await adminAPI('/api/rules')) } catch (failure) { error.value = failure.message }
  finally { busy.value = false }
}
async function save() {
  busy.value = true; error.value = ''; feedback.value = ''
  try {
    accept(await adminAPI('/api/rules', { method: 'PUT', body: JSON.stringify(draft.value) }))
    feedback.value = 'Regras publicadas. O site já mostra a nova versão ao abrir ou atualizar a página.'
  } catch (failure) { error.value = failure.message; conflict.value = failure.status === 409 }
  finally { busy.value = false }
}
function reload() { if (dirty.value) confirmReload.value = true; else load() }
function addTopic(clan) { clan.topics.push({ id: clan.anchor + '-' + crypto.randomUUID(), title: '', items: [''] }) }
function move(clan, index, direction) { [clan.topics[index], clan.topics[index + direction]] = [clan.topics[index + direction], clan.topics[index]] }
function beforeLeaving(event) { if (dirty.value) { event.preventDefault(); event.returnValue = '' } }
onMounted(() => { load(); window.addEventListener('beforeunload', beforeLeaving) })
onBeforeUnmount(() => window.removeEventListener('beforeunload', beforeLeaving))
</script>
<template>
  <section class="admin-editor">
    <div class="section"><div><small>CONTEÚDO PÚBLICO</small><h2>Regras da comunidade</h2><p>Edite os textos e links. Salvar publica as alterações na página inicial.</p></div><a href="https://imperiobarbaro.com.br/#regras" target="_blank" rel="noopener noreferrer">Ver regras públicas ↗</a></div>
    <p v-if="error" class="notice error" role="alert">{{ error }}</p>
    <p v-if="feedback && !dirty" class="notice" role="status">{{ feedback }}</p>
    <p v-if="busy" role="status">Carregando ou salvando regras…</p>
    <button v-if="!draft && !busy" class="secondary" @click="load">Tentar novamente</button>
    <form v-if="draft" @submit.prevent="save">
      <div class="editor-actions rules-publish-bar"><button class="primary" :disabled="busy || !dirty || conflict">{{ busy ? 'Salvando…' : 'Salvar e publicar regras' }}</button><button type="button" class="secondary" :disabled="busy" @click="reload">Recarregar regras salvas</button></div>
      <p class="report-note" role="status">{{ dirty ? 'Alterações ainda não publicadas.' : 'Exibindo as regras publicadas.' }} Os links por tópico são preservados ao editar; remover um tópico também remove seu link.</p>
      <fieldset :disabled="busy">
        <legend>Grupo geral</legend>
        <label>Título<input v-model="draft.general.title" maxlength="150" required></label>
        <label>Orientação sobre punições<input v-model="draft.general.penalty" maxlength="300"></label>
        <RuleLines v-model="draft.general.items" label="Regra geral"/>
      </fieldset>
      <fieldset v-for="clan in draft.clans" :key="clan.id" :disabled="busy">
        <legend>{{ clan.id === '1' ? 'Clã principal' : 'Clã secundário' }}</legend>
        <label>Nome do clã<input v-model="clan.name" maxlength="100" required></label>
        <div class="editor-grid">
          <label>Resumo das guerras<input v-model="clan.warSummary" maxlength="200" required></label>
          <label>Resumo de inatividade<input v-model="clan.inactivitySummary" maxlength="200" required></label>
        </div>
        <label>Link do regulamento ou PDF <span>(opcional)</span><input v-model="clan.documentUrl" type="url" pattern="https://.*" maxlength="2048" placeholder="https://…"></label>
        <p class="report-note">Use um link HTTPS público, como um PDF no Drive com acesso de leitura. Deixe vazio para ocultar o link. Não é necessário enviar um arquivo ao aplicativo.</p>
        <details v-for="(topic, index) in clan.topics" :key="topic.id" class="rule-topic-editor"><summary>{{ index + 1 }}. {{ topic.title || 'Novo tópico' }}</summary>
          <div class="editor-actions"><strong>Tópico {{ index + 1 }}</strong><button type="button" class="secondary" :disabled="index === 0" @click="move(clan, index, -1)" aria-label="Mover tópico para cima">↑</button><button type="button" class="secondary" :disabled="index === clan.topics.length - 1" @click="move(clan, index, 1)" aria-label="Mover tópico para baixo">↓</button><button type="button" class="secondary" :disabled="clan.topics.length === 1" @click="clan.topics.splice(index, 1)">Remover tópico</button></div>
          <label>Título do tópico<input v-model="topic.title" maxlength="150" required></label>
          <a class="topic-link" :href="'https://imperiobarbaro.com.br/#' + topic.id" target="_blank" rel="noopener noreferrer">Link permanente deste tópico ↗</a>
          <RuleLines v-model="topic.items" label="Orientação"/>
        </details>
        <button type="button" class="secondary" :disabled="clan.topics.length >= 20" @click="addTopic(clan)">＋ Adicionar tópico</button>
      </fieldset>
      <div v-if="confirmReload" class="notice"><p>Descartar as alterações ainda não salvas e carregar as regras publicadas?</p><div class="editor-actions"><button class="secondary" type="button" @click="load">Descartar e recarregar</button><button class="secondary" type="button" @click="confirmReload = false">Continuar editando</button></div></div>
    </form>
  </section>
</template>
