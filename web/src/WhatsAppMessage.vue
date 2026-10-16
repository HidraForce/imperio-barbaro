<script setup>
import { ref, watch } from 'vue'
const props = defineProps({title:String,message:String,busy:Boolean,empty:{type:String,default:'Nenhuma mensagem disponível.'}})
const status=ref(''),field=ref(null)
watch(()=>props.message,()=>{status.value=''})
async function copy(){
  try{await navigator.clipboard.writeText(props.message);status.value='Mensagem copiada. Cole no WhatsApp.'}
  catch{field.value?.focus();field.value?.select();status.value='Texto selecionado. Use copiar e cole no WhatsApp.'}
}
</script>
<template>
  <section class="message-generator">
    <div class="section"><div><small>MENSAGEM PARA WHATSAPP</small><h2>{{ title }}</h2></div></div>
    <slot/>
    <template v-if="message"><textarea ref="field" :value="message" readonly rows="12" :aria-label="title+' · mensagem para WhatsApp'"></textarea><button type="button" class="primary" :disabled="busy" @click="copy">Copiar mensagem para WhatsApp</button></template>
    <p v-else class="empty">{{ empty }}</p>
    <p v-if="status" class="report-note" role="status">{{ status }}</p>
  </section>
</template>
