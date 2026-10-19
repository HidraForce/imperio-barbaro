<script setup>
import { inject, ref } from 'vue'
const vault=inject('contactVault')
const {password,unlocked,busy,error,unlock,lock}=vault
const enabled=defineModel({type:Boolean,default:true})
const showUnlock=ref(false)
</script>
<template>
  <section class="contact-message-access">
    <template v-if="unlocked"><label class="check-label"><input type="checkbox" v-model="enabled"> Incluir @número cadastrado ao lado dos nicknames nas mensagens</label><button class="secondary" @click="lock()">Bloquear números</button></template>
    <template v-else><p>Números protegidos. Desbloqueie com a senha de Contatos para incluí-los nas mensagens.</p><button v-if="!showUnlock" class="secondary" @click="showUnlock=true">Desbloquear números para mensagens</button><form v-else @submit.prevent="unlock"><label>Senha de Contatos<input v-model="password" type="password" autocomplete="off" maxlength="1024" :disabled="busy" required></label><button class="primary" :disabled="busy">{{ busy?'Desbloqueando…':'Desbloquear números' }}</button></form></template>
    <p v-if="error" class="notice error" role="alert">{{ error }}</p>
    <p class="report-note">O desbloqueio vale nas abas do painel por até 10 minutos e termina ao ocultar ou sair da página. O texto @número ajuda a identificar a pessoa; no WhatsApp, selecione o participante na lista de @ para confirmar a menção.</p>
  </section>
</template>
