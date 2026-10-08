<script setup>
import { computed, ref, watch } from 'vue'
import { pendingMessage } from './battle-report.js'
const props=defineProps({clanName:String,clanTag:String,items:{type:Array,default:()=>[]},busy:Boolean,context:String})
const player=ref(''),copyStatus=ref(''),messageField=ref(null)
const people=computed(()=>[...new Map(props.items.map(item=>[item.tag,{tag:item.tag,name:item.name}])).values()].sort((a,b)=>a.name.localeCompare(b.name,'pt-BR')))
const message=computed(()=>pendingMessage({clanName:props.clanName,clanTag:props.clanTag,items:props.items,playerTag:player.value}))
watch(()=>[props.clanTag,props.context],()=>{player.value='';copyStatus.value=''})
watch(message,()=>{copyStatus.value='';if(player.value&&!people.value.some(p=>p.tag===player.value))player.value=''})
async function copy(){try{await navigator.clipboard.writeText(message.value);copyStatus.value='Mensagem copiada. Cole no WhatsApp.'}catch{messageField.value?.focus();messageField.value?.select();copyStatus.value='Selecionei o texto. Use copiar e cole no WhatsApp.'}}
</script>
<template>
  <section class="message-generator">
    <div class="section"><div><small>MENSAGEM PARA WHATSAPP · SOMENTE ESTE CLÃ</small><h2>Gerador de inadimplência</h2><p>{{ clanName }} · #{{ clanTag }} · {{ context }}</p></div></div>
    <p class="report-note">O texto é gerado pelos registros disponíveis. Atividades encerradas pedem justificativa e avisam que, sem ela, o jogador ficará sob aviso. Atividades abertas recebem um lembrete.</p>
    <label class="message-recipient">Destinatários<select v-model="player" :disabled="busy"><option value="">Todos com pendências deste clã</option><option v-for="person in people" :key="person.tag" :value="person.tag">{{ person.name }} · {{ person.tag }}</option></select></label>
    <p v-if="busy" class="loading">Aguarde a consulta para gerar a mensagem.</p>
    <template v-else-if="message"><textarea ref="messageField" :value="message" readonly rows="12" aria-label="Mensagem pronta para copiar e enviar pelo WhatsApp"></textarea><button class="primary" @click="copy">Copiar mensagem para WhatsApp</button></template>
    <p v-else class="empty">Nenhuma pendência identificada nos dados disponíveis deste recorte.</p>
    <p v-if="copyStatus" role="status" class="report-note">{{ copyStatus }}</p>
  </section>
</template>
