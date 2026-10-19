<script setup>
import { inject, ref } from 'vue'
const vault=inject('contactVault')
const {password,unlocked,busy,error,unlock,lock}=vault
const enabled=defineModel({type:Boolean,default:false})
const showUnlock=ref(false)
</script>
<template>
  <section class="contact-message-access">
    <fieldset class="message-contact-options"><legend>Formato das mensagens automáticas</legend><label><input type="radio" name="message-contact-format" v-model="enabled" :value="false"> Sem contato · somente nickname e tag</label><label><input type="radio" name="message-contact-format" v-model="enabled" :value="true" :disabled="!unlocked"> Com contato · nickname e @número sem o 55 do Brasil</label></fieldset>
    <p class="report-note">A opção escolhida vale para mensagens de ataques, avisos e punições. O telefone salvo em Contatos não é alterado.</p>
    <template v-if="unlocked"><button class="secondary" @click="lock()">Bloquear números</button></template>
    <template v-else><p>Números protegidos. Desbloqueie com a senha de Contatos para incluí-los nas mensagens.</p><button v-if="!showUnlock" class="secondary" @click="showUnlock=true">Desbloquear números para mensagens</button><form v-else @submit.prevent="unlock"><label>Senha de Contatos<input v-model="password" type="password" autocomplete="off" maxlength="1024" :disabled="busy" required></label><button class="primary" :disabled="busy">{{ busy?'Desbloqueando…':'Desbloquear números' }}</button></form></template>
    <p v-if="error" class="notice error" role="alert">{{ error }}</p>
    <p class="report-note">O desbloqueio vale nas abas do painel por até 10 minutos e termina ao ocultar ou sair da página. O texto @número ajuda a identificar a pessoa; no WhatsApp, selecione o participante na lista de @ para confirmar a menção.</p>
  </section>
</template>
