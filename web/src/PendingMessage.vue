<script setup>
import { computed, ref, watch } from 'vue'
import { filterBattleWarnings, pendingMessage } from './battle-report.js'
const props=defineProps({clanName:String,clanTag:String,items:{type:Array,default:()=>[]},busy:Boolean,context:String,warMode:Boolean,phones:{type:Object,default:()=>({})}})
const player=ref(''),attackFilter=ref('all'),copyStatus=ref(''),messageField=ref(null)
const warningCounts=computed(()=>Object.fromEntries(['all','0','1'].map(filter=>[filter,new Set(filterBattleWarnings(props.items,filter).map(item=>item.tag)).size])))
const filteredItems=computed(()=>props.warMode?filterBattleWarnings(props.items,attackFilter.value):props.items)
const people=computed(()=>[...new Map(filteredItems.value.map(item=>[item.tag,{tag:item.tag,name:item.name}])).values()].sort((a,b)=>a.name.localeCompare(b.name,'pt-BR')))
const message=computed(()=>pendingMessage({clanName:props.clanName,clanTag:props.clanTag,items:filteredItems.value,playerTag:player.value,phones:props.phones}))
watch(()=>[props.clanTag,props.context,props.warMode],()=>{player.value='';attackFilter.value='all';copyStatus.value=''})
watch(attackFilter,()=>{player.value='';copyStatus.value=''})
watch(people,()=>{if(player.value&&!people.value.some(p=>p.tag===player.value))player.value=''})
watch(message,()=>{copyStatus.value=''})
async function copy(){try{await navigator.clipboard.writeText(message.value);copyStatus.value='Mensagem copiada. Cole no WhatsApp.'}catch{messageField.value?.focus();messageField.value?.select();copyStatus.value='Selecionei o texto. Use copiar e cole no WhatsApp.'}}
</script>
<template>
  <section class="message-generator">
    <div class="section"><div><small>MENSAGEM PARA WHATSAPP · SOMENTE ESTE CLÃ</small><h2>{{ warMode ? 'Avisos de ataques da guerra' : 'Gerador de inadimplência' }}</h2><p>{{ clanName }} · #{{ clanTag }} · {{ context }}</p></div></div>
    <p class="report-note">O texto é gerado pelos registros disponíveis. Atividades encerradas pedem justificativa e avisam que, sem ela, o jogador ficará sob aviso. Atividades abertas recebem um lembrete.</p>
    <label v-if="warMode" class="message-recipient">Ataques realizados<select v-model="attackFilter" :disabled="busy"><option value="all">Todos com ataques pendentes ({{ warningCounts.all }})</option><option value="0">0 ataques ({{ warningCounts['0'] }})</option><option value="1">1 ataque e ainda com pendência ({{ warningCounts['1'] }})</option></select></label>
    <label class="message-recipient">Destinatários<select v-model="player" :disabled="busy"><option value="">{{ warMode ? 'Todos deste filtro' : 'Todos com pendências deste clã' }}</option><option v-for="person in people" :key="person.tag" :value="person.tag">{{ person.name }} · {{ person.tag }}</option></select></label>
    <p v-if="busy" class="loading">Aguarde a consulta para gerar a mensagem.</p>
    <template v-else-if="message"><textarea ref="messageField" :value="message" readonly rows="12" aria-label="Mensagem pronta para copiar e enviar pelo WhatsApp"></textarea><button class="primary" @click="copy">Copiar mensagem para WhatsApp</button></template>
    <p v-else class="empty">Nenhuma pendência identificada nos dados disponíveis deste recorte.</p>
    <p v-if="copyStatus" role="status" class="report-note">{{ copyStatus }}</p>
  </section>
</template>
