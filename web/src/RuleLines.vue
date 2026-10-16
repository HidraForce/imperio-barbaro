<script setup>
const items = defineModel({ type: Array, required: true })
defineProps({ label: String })
function move(index, direction) {
  const list = [...items.value]
  ;[list[index], list[index + direction]] = [list[index + direction], list[index]]
  items.value = list
}
</script>
<template>
  <div class="rule-lines">
    <div v-for="(_, index) in items" :key="index" class="rule-line">
      <label>{{ label }} {{ index + 1 }}<textarea v-model="items[index]" maxlength="1500" required rows="3"></textarea></label>
      <div class="editor-actions">
        <button type="button" class="secondary" :disabled="index === 0" :aria-label="'Mover regra ' + (index + 1) + ' para cima'" @click="move(index, -1)">↑</button>
        <button type="button" class="secondary" :disabled="index === items.length - 1" :aria-label="'Mover regra ' + (index + 1) + ' para baixo'" @click="move(index, 1)">↓</button>
        <button type="button" class="secondary" :disabled="items.length === 1" @click="items.splice(index, 1)">Remover regra</button>
      </div>
    </div>
    <button type="button" class="secondary" :disabled="items.length >= 30" @click="items.push('')">＋ Adicionar regra</button>
  </div>
</template>
