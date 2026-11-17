<script setup>
import { onBeforeUnmount, onDeactivated, onMounted } from 'vue'
import { createScopedVault } from './announcement-vault.js'

const props = defineProps({
  area: { type: String, required: true },
  title: { type: String, required: true },
  editor: { type: Object, required: true },
  clans: { type: Array, required: true },
})
const vault = createScopedVault('/api/' + props.area, 'X-Editor-Session')

function lock() { vault.lock() }
function lockWhenHidden() { if (document.hidden) lock() }

onMounted(() => {
  document.addEventListener('visibilitychange', lockWhenHidden)
  window.addEventListener('pagehide', lock)
})
onDeactivated(lock)
onBeforeUnmount(() => {
  document.removeEventListener('visibilitychange', lockWhenHidden)
  window.removeEventListener('pagehide', lock)
  lock()
})
</script>

<template>
  <section>
    <template v-if="!vault.unlocked.value">
      <div class="section"><h2>{{ title }}</h2></div>
      <form class="admin-editor editor-unlock" @submit.prevent="vault.unlock">
        <label>Senha de edição
          <input v-model="vault.password.value" type="password" autocomplete="off" required :disabled="vault.busy.value">
        </label>
        <button class="primary" :disabled="vault.busy.value">{{ vault.busy.value ? 'Desbloqueando…' : 'Desbloquear edição' }}</button>
        <p v-if="vault.error.value" class="notice error" role="alert">{{ vault.error.value }}</p>
      </form>
    </template>
    <div v-else class="editor-lock"><button type="button" class="secondary" @click="lock">Bloquear edição</button></div>
    <KeepAlive>
      <component v-if="vault.unlocked.value" :is="editor" :clans="clans" :request="vault.request"/>
    </KeepAlive>
  </section>
</template>

<style scoped>
.editor-unlock{max-width:420px}
.editor-lock{display:flex;justify-content:flex-end;margin-bottom:12px}
</style>
