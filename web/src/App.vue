<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { buildAttackOverview } from './attack-overview.js'
import { csvCell } from './report-csv.js'
import UiIcon from './UiIcon.vue'
import ContactsPanel from './ContactsPanel.vue'
import PendingMessage from './PendingMessage.vue'
import LeadershipPanel from './LeadershipPanel.vue'
import { ownSide, seasonRows, battleCSV, battlePending, raidPending } from './battle-report.js'
const tab=ref('Overview'), clan=ref(null), war=ref(null),  config=ref({configured:false,clans:[]}), selectedClan=ref('1'), loading=ref(true), error=ref(''), warError=ref(''), raidError=ref('')
const leagueGroup=ref(null), leagueWar=ref(null), leagueLoading=ref(false), leagueWarLoading=ref(false), leagueError=ref(''), leagueMessage=ref(''), roundMessage=ref(''), roundError=ref(''), selectedRound=ref(1)
let leagueRequest=0, roundRequest=0, allRoundsRequest=0
const leagueView=ref('round'), allRounds=ref([]), allRoundsLoading=ref(false)
const seasonPlayers=computed(()=>seasonRows(leagueGroup.value,allRounds.value,activeClanTag.value))
const leagueRegistered=computed(()=>leagueGroup.value?.clans?.find(c=>c.tag?.replace(/^#/,'').toUpperCase()===activeClanTag.value.toUpperCase())?.members||[])
const leagueOurSide=computed(()=>{const w=leagueWar.value;if(!w)return null;return w.clan?.tag?.replace(/^#/, '').toUpperCase()===activeClanTag.value.toUpperCase()?w.clan:w.opponent})
const leagueEnemySide=computed(()=>!leagueWar.value?null:leagueOurSide.value===leagueWar.value.clan?leagueWar.value.opponent:leagueWar.value.clan)
const leagueMembers=computed(()=>[...(leagueOurSide.value?.members||[])].sort((a,b)=>(a.mapPosition||0)-(b.mapPosition||0)))
const leagueMissing=computed(()=>leagueMembers.value.filter(m=>!m.attacks?.length))
function stateLabel(value){return ({preparation:'Preparação',inWar:'Dia de batalha',warEnded:'Guerra encerrada',inProgress:'Em andamento',ended:'Liga encerrada',notInWar:'Fora de guerra',pending:'Pendente'})[value]||value||'Indisponível'}
function roundAssigned(round){return (round?.warTags||[]).some(t=>t&&t!=='#0'&&t!=='0')}
async function loadLeague(){
 const request=++leagueRequest;roundRequest++;const clanId=selectedClan.value
 allRoundsRequest++;allRounds.value=[];allRoundsLoading.value=false;leagueView.value='round';leagueLoading.value=true;leagueWarLoading.value=false;leagueError.value='';leagueMessage.value='';roundError.value='';roundMessage.value='';leagueGroup.value=null;leagueWar.value=null
 try{
  const result=await api('/api/cwl?clan='+clanId)
  if(request!==leagueRequest||clanId!==selectedClan.value)return
  leagueGroup.value=result.group||null;leagueMessage.value=result.message||''
  if(leagueGroup.value){
   const rounds=leagueGroup.value.rounds||[]
   selectedRound.value=Math.max(1,rounds.reduce((last,r,i)=>roundAssigned(r)?i+1:last,1))
   if(rounds.length)await loadLeagueRound()
  }
 }catch(e){if(request===leagueRequest)leagueError.value=e.message}finally{if(request===leagueRequest)leagueLoading.value=false}
}
async function loadLeagueRound(){
 const request=++roundRequest;const clanId=selectedClan.value;const round=selectedRound.value
 leagueWarLoading.value=true;roundError.value='';roundMessage.value='';leagueWar.value=null
 allRounds.value=allRounds.value.filter(entry=>entry.round!==round)
 try{
  const result=await api('/api/cwl/round?clan='+clanId+'&round='+round)
  if(request!==roundRequest||clanId!==selectedClan.value)return
  leagueWar.value=result.war||null;roundMessage.value=result.message||''
  allRounds.value=[...allRounds.value.filter(entry=>entry.round!==round),{round,war:result.war||null,message:result.message||''}].sort((a,b)=>a.round-b.round)
 }catch(e){if(request===roundRequest){roundError.value=e.message;allRounds.value=[...allRounds.value.filter(entry=>entry.round!==round),{round,war:null,error:e.message}].sort((a,b)=>a.round-b.round)}}finally{if(request===roundRequest)leagueWarLoading.value=false}
}
async function loadAllRounds(){
 leagueView.value='all'
 const request=++allRoundsRequest, clanId=selectedClan.value
 allRoundsLoading.value=true
 const results=await Promise.allSettled((leagueGroup.value?.rounds||[]).map(async (round,index)=>{
  const number=index+1
  if(!roundAssigned(round))return {round:number,war:null,message:'Rodada ainda não definida'}
  const saved=allRounds.value.find(entry=>entry.round===number&&entry.war)
  if(saved)return saved
  const result=await api('/api/cwl/round?clan='+clanId+'&round='+number)
  return {round:number,war:result.war||null,message:result.message||''}
 }))
 if(request!==allRoundsRequest||clanId!==selectedClan.value)return
 allRounds.value=results.map((result,index)=>result.status==='fulfilled'?result.value:{round:index+1,war:null,error:result.reason.message})
 allRoundsLoading.value=false
}
function chooseRound(number){leagueView.value='round';selectedRound.value=number;loadLeagueRound()}
watch(tab,value=>{if(['Overview','CWL'].includes(value)&&config.value.configured&&!loading.value&&!leagueGroup.value&&!leagueLoading.value)loadLeague()})
const raidRecords=ref([]), selectedWeekend=ref(''), raidFilter=ref('all')
const tabs=['Overview','Raids','CWL','War','Roster','Contacts','Leadership']
const tabLabels={Overview:'Visão geral',Raids:'Raids',CWL:'Liga',War:'Guerra',Roster:'Membros',Contacts:'Contatos',Leadership:'Liderança'}
function roleLabel(role){return ({leader:'Líder',coLeader:'Colíder',admin:'Ancião',member:'Membro'})[role]||'Membro'}
const members=computed(()=>clan.value?.memberList||[]), warMembers=computed(()=>war.value?.clan?.members||[])
const activeClanTag=computed(()=>config.value.clans?.find(c=>c.id===selectedClan.value)?.tag||'')
const warState=computed(()=>!war.value?'Indisponível':war.value.state==='notInWar'?'Fora de guerra':war.value.state==='warEnded'?'Guerra encerrada':war.value.state==='preparation'?'Preparação':'Em andamento')
const raidRecord=computed(()=>raidRecords.value.find(r=>r.season.startTime===selectedWeekend.value)||null)
const raid=computed(()=>raidRecord.value?.season||null)
const raidEnded=computed(()=>!!raid.value && raidDate(raid.value.endTime).getTime()<=Date.now())
const raidPlayers=computed(()=>{
 const attackers=new Map((raid.value?.members||[]).map(m=>[m.tag,m]))
 const roster=raidRecord.value?.roster||[]
 const rosterTags=new Set(roster.map(m=>m.tag))
 const rows=roster.map(m=>({...m,...(attackers.get(m.tag)||{}),attacks:attackers.get(m.tag)?.attacks||0,inRoster:true}))
 for(const m of attackers.values())if(!rosterTags.has(m.tag))rows.push({...m,inRoster:false})
 return rows.sort((a,b)=>a.attacks-b.attacks||a.name.localeCompare(b.name))
})
const nonAttackers=computed(()=>raidPlayers.value.filter(m=>m.inRoster&&m.attacks===0))
const visibleRaidPlayers=computed(()=>raidPlayers.value.filter(m=>raidFilter.value==='all'||(raidFilter.value==='missing'?m.inRoster&&m.attacks===0:m.attacks>0)))
const attackingCount=computed(()=>raidPlayers.value.filter(m=>m.attacks>0).length)
const onlyPending=ref(false)
const overviewRaidRecord=computed(()=>raidRecords.value[0]||null)
const overviewColumns=[{key:'raids',label:'Raids'},{key:'war',label:'Guerra'},{key:'cwl',label:'CWL'}]
const overviewRows=computed(()=>buildAttackOverview({members:members.value,raidRecord:overviewRaidRecord.value,war:war.value,leagueWar:leagueWar.value,leagueMembers:leagueMembers.value,loading:loading.value,leagueLoading:leagueLoading.value||leagueWarLoading.value,warError:warError.value,leagueError:leagueError.value||roundError.value}))
const visibleOverviewRows=computed(()=>overviewRows.value.filter(row=>!onlyPending.value||row.pendingCount>0))
const pendingPlayers=computed(()=>overviewRows.value.filter(row=>row.pendingCount>0).length)

function raidDate(value){if(!value)return new Date(NaN);return new Date(value.replace(/^(\d{4})(\d{2})(\d{2})T(\d{2})(\d{2})(\d{2})\.000Z$/,'$1-$2-$3T$4:$5:$6Z'))}
function dateLabel(value){const d=raidDate(value);return Number.isNaN(d.getTime())?'Data desconhecida':d.toLocaleDateString('pt-BR',{day:'numeric',month:'short',year:'numeric'})}
function number(value){return Number(value||0).toLocaleString('pt-BR')}
function allowance(m){return m.attackLimit==null?null:m.attackLimit+(m.bonusAttackLimit||0)}
function raidStatus(m){if(!m.attacks)return raidEnded.value?'Sem ataques registrados':'Ainda não atacou';return allowance(m)!=null&&m.attacks>=allowance(m)?'Todos os ataques usados':'Atacou'}
async function api(path,options={}){const headers=new Headers(options.headers);headers.set('X-Clanboard-Request','1');const r=await fetch(path,{...options,headers,credentials:'same-origin',cache:'no-store'});if(!r.ok){const b=await r.json().catch(()=>({}));throw Error(b.error||b.message||('Falha na consulta ('+r.status+')'))}return r.status===204?null:r.json()}
async function load(){
 if(loading.value&&config.value.clans.length)return
 leagueRequest++;roundRequest++;allRoundsRequest++;allRounds.value=[];allRoundsLoading.value=false;leagueView.value='round';leagueGroup.value=null;leagueWar.value=null;leagueLoading.value=false;leagueWarLoading.value=false;leagueError.value='';leagueMessage.value='';roundError.value='';roundMessage.value='';loading.value=true;error.value='';warError.value='';raidError.value='';clan.value=null;war.value=null;raidRecords.value=[]
 try{
  config.value=await api('/api/config')
  if(!config.value.clans?.some(c=>c.id===selectedClan.value&&c.configured))selectedClan.value=config.value.clans?.find(c=>c.configured)?.id||'1'
  if(config.value.configured){
   const suffix='?clan='+selectedClan.value
   const results=await Promise.allSettled([api('/api/clan'+suffix),api('/api/war'+suffix),api('/api/raids'+suffix)])
   if(results[0].status==='fulfilled')clan.value=results[0].value;else error.value=results[0].reason.message
   if(results[1].status==='fulfilled')war.value=results[1].value;else warError.value=results[1].reason.message
   if(results[2].status==='fulfilled'){
    raidRecords.value=results[2].value.items||[];raidError.value=results[2].value.warning||''
    if(!raidRecords.value.some(r=>r.season.startTime===selectedWeekend.value))selectedWeekend.value=raidRecords.value[0]?.season.startTime||''
   }else raidError.value=results[2].reason.message
  }
 }catch(e){error.value=e.message}finally{loading.value=false;if(['Overview','CWL'].includes(tab.value)&&config.value.configured)await loadLeague()}
}
async function chooseClan(id){if(loading.value||selectedClan.value===id)return;selectedClan.value=id;selectedWeekend.value='';await load()}
function stars(m){return m.attacks?.reduce((n,a)=>n+a.stars,0)||0}function percent(m){return m.attacks?.reduce((n,a)=>n+a.destructionPercentage,0)||0}
function downloadReport(format){
 if(!raidRecord.value)return
 const record=raidRecord.value, season=raid.value
 const title=`${clan.value?.name||'Clã'} · #${activeClanTag.value} · Raid ${dateLabel(season.startTime)} – ${dateLabel(season.endTime)}`
 const basis=`Lista de membros registrada em ${new Date(record.rosterCapturedAt).toLocaleString('pt-BR')} (${record.rosterDuringRaid?'durante esta raid':'após esta raid; a composição histórica do clã não foi confirmada'}).`
 const status=raidEnded.value?'Fim de semana concluído':'PROVISÓRIO — raid ainda em andamento'
 const note='Sem ataques registrados neste clã. A lista pode não incluir membros que entraram ou saíram entre sincronizações; confira a participação e a elegibilidade antes de tomar medidas.'
 let content
 if(format==='csv'){
  const rows=[['Relatório semanal de raids',title],['Situação',status],['Referência da lista de membros',basis],['Última consulta de raids',record.syncedAt],['Observação',note],[],['Jogador','Tag','Ataques','Situação','Lista registrada']]
  for(const m of raidPlayers.value)rows.push([m.name,m.tag,m.attacks,raidStatus(m),m.inRoster?'Incluído':'Fora da lista registrada'])
  content='\uFEFF'+rows.map(row=>row.map(csvCell).join(',')).join('\r\n')
 }else{
  content=[title,status,basis,`Última consulta de raids: ${new Date(record.syncedAt).toLocaleString('pt-BR')}`,note,'',`MEMBROS SEM ATAQUES REGISTRADOS (${nonAttackers.value.length})`,...nonAttackers.value.map(m=>`• ${m.name} (${m.tag})`),...(nonAttackers.value.length?[]:['Nenhum.']), '',`ATACANTES (${attackingCount.value})`,...raidPlayers.value.filter(m=>m.attacks>0).map(m=>`${m.name} (${m.tag}) — ${m.attacks} ataques, ${number(m.capitalResourcesLooted)} ouro da capital`)].join('\n')
 }
 const url=URL.createObjectURL(new Blob([content],{type:format==='csv'?'text/csv;charset=utf-8':'text/plain;charset=utf-8'}))
 const link=document.createElement('a');link.href=url;link.download=`raids-${activeClanTag.value}-${season.startTime.slice(0,8)}.${format}`;link.click();setTimeout(()=>URL.revokeObjectURL(url),1000)
}
function downloadBattle(type){
 const isLeague=type==='CWL'
 const rounds=isLeague?(leagueView.value==='all'?allRounds.value:[{round:selectedRound.value,war:leagueWar.value}]):[{war:war.value}]
 const content=battleCSV({clanName:clan.value?.name,clanTag:activeClanTag.value,type,season:isLeague?leagueGroup.value?.season:'',rounds,registered:isLeague&&leagueView.value==='all'?leagueRegistered.value:[]})
 const url=URL.createObjectURL(new Blob([content],{type:'text/csv;charset=utf-8'}))
 const link=document.createElement('a');link.href=url;link.download=`${isLeague?'cwl':'guerra'}-${activeClanTag.value}-${isLeague?(leagueGroup.value?.season||'')+'-'+(leagueView.value==='all'?'todos':'rodada-'+selectedRound.value):'atual'}.csv`;link.click();setTimeout(()=>URL.revokeObjectURL(url),1000)
}
const messageContext=computed(()=>tab.value==='Raids'?'Raids selecionadas':tab.value==='War'?'Guerra atual':tab.value==='CWL'?(leagueView.value==='all'?'CWL · temporada completa':'CWL · rodada '+selectedRound.value):'Raids recentes, guerra atual e CWL · rodada '+selectedRound.value)
const messageBusy=computed(()=>loading.value||(['Overview','CWL'].includes(tab.value)&&(leagueLoading.value||leagueWarLoading.value||(leagueView.value==='all'&&allRoundsLoading.value))))
const messageItems=computed(()=>{
 const items=[]
 if(['Overview','Raids'].includes(tab.value)&&!raidError.value)items.push(...raidPending(tab.value==='Raids'?raidRecord.value:overviewRaidRecord.value))
 if(['Overview','War'].includes(tab.value)&&!warError.value)items.push(...battlePending({war:war.value},activeClanTag.value,'Guerra atual',2))
 if(['Overview','CWL'].includes(tab.value)&&!leagueError.value){
  const entries=tab.value==='CWL'&&leagueView.value==='all'?allRounds.value:roundError.value?[]:[{round:selectedRound.value,war:leagueWar.value}]
  for(const entry of entries)items.push(...battlePending(entry,activeClanTag.value,'CWL · rodada '+entry.round))
 }
 return items
})
onMounted(load)
</script>
<template>
<div class="shell">
  <aside class="sidebar"><div class="brand"><span class="brand-icon"><UiIcon name="Shield"/></span> IMPÉRIO<span class="light"> BÁRBARO CRM</span></div><div class="label">SEU CLÃ</div><div class="clan"><span class="crest"><UiIcon name="War"/></span><span><b>{{ clan?.name||'Painel do clã' }}</b><small>{{ activeClanTag?'#'+activeClanTag:'Selecione um clã configurado' }}</small></span><i class="dot"></i></div><div class="label nav-label">GESTÃO</div><button v-for="item in tabs" :key="item" class="nav" :class="{active:tab===item}" @click="tab=item"><UiIcon :name="item"/>{{ tabLabels[item] }}</button><div class="sidebar-foot"><i class="dot"></i> Conexão com a API <b>{{ config.configured?'Configurada':'Configuração necessária' }}</b><button @click="load">↻ &nbsp;Atualizar dados</button></div></aside>
  <main><header><span>GESTÃO DO CLÃ <span class="header-section">&nbsp; / &nbsp; <b>{{ tabLabels[tab].toUpperCase() }}</b></span></span><span class="live"><i class="dot"></i> DADOS DO CLÃ</span></header><div class="content">
    <div class="heading"><div><small>PAINEL DO CLÃ</small><h1>{{ tab==='Overview'?'Controle de ataques':tab==='CWL'?'Liga de Guerra de Clãs':tabLabels[tab] }}</h1><p>{{ clan?.name?clan.name+' · #'+activeClanTag:'Acompanhe seu clã em um só lugar.' }}</p></div><div class="heading-actions" v-if="tab!=='Leadership'"><div class="clan-switch" v-if="config.clans?.some(c=>c.configured)"><button v-for="c in config.clans" :key="c.id" :disabled="!c.configured||loading" :class="{selected:selectedClan===c.id}" @click="chooseClan(c.id)">Clã {{ c.id }}</button></div><button class="primary" :disabled="loading" @click="load">↻ &nbsp;Sincronizar</button></div></div>
    <p v-if="!['Overview','Contacts','Leadership'].includes(tab)" class="report-note">Cache compartilhado: clã por 5 minutos, guerras e liga por 1 minuto e raids por 15 minutos. Sincronizar reutiliza os dados dentro desse prazo.</p>
    <div v-if="error" class="notice error">{{ error }} <button @click="error=''">×</button></div><div v-if="!config.configured&&tab!=='Leadership'" class="notice">⌁ &nbsp; Configure <code>CLASH_API_TOKEN</code>, <code>CLAN_TAG1</code> e <code>CLAN_TAG2</code> no .env e reinicie o aplicativo.</div><p v-if="loading" class="loading">Carregando dados do clã…</p>
    <template v-if="tab==='Overview'">
      <div class="attack-toolbar">
        <div><h2>Quem precisa atacar <em>{{ pendingPlayers }} jogadores</em></h2><p>Pendências primeiro. Verde: todos os ataques usados. Vermelho: faltam ataques. Cinza: sem obrigação confirmada ou sem dados.</p></div>
        <label class="pending-toggle"><input type="checkbox" v-model="onlyPending"> Só pendências</label>
      </div>
      <div class="attack-context">
        <span><b>Raids</b> {{ overviewRaidRecord ? dateLabel(overviewRaidRecord.season.startTime) : 'Sem fim de semana disponível' }}</span>
        <span><b>Guerra</b> {{ warState }}</span>
        <label><b>CWL</b><select v-if="leagueGroup?.rounds?.length" v-model="selectedRound" :disabled="leagueLoading||leagueWarLoading||loading" @change="loadLeagueRound"><option v-for="(_,index) in leagueGroup.rounds" :key="index" :value="index+1">Rodada {{ index+1 }}</option></select><span v-else>{{ leagueLoading?'Carregando…':'Sem liga atual' }}</span></label>
      </div>
      <div v-if="warError" class="notice error">Guerra: {{ warError }}</div>
      <div v-if="raidError" class="notice error">Raids: {{ raidError }} <span v-if="overviewRaidRecord">Exibindo registros salvos.</span></div>
      <div v-if="leagueError||roundError" class="notice error">CWL: {{ leagueError||roundError }}</div>
      <p v-if="roundMessage||leagueMessage" class="loading">{{ roundMessage||leagueMessage }}</p>
      <section class="table-wrap attack-table-wrap">
        <table class="attack-table">
          <caption class="sr-only">Ataques por jogador nas raids mais recentes, na guerra atual e na rodada selecionada da CWL</caption>
          <thead><tr><th scope="col">JOGADOR</th><th v-for="column in overviewColumns" :key="column.key" :class="'activity-'+column.key" scope="col">{{ column.label }}</th></tr></thead>
          <tbody>
            <tr v-for="player in visibleOverviewRows" :key="player.tag" :class="{'has-pending':player.pendingCount>0}">
              <th scope="row"><b>{{ player.name }}</b><small>{{ player.tag }}{{ player.current?'':' · Fora do clã atual' }}</small></th>
              <td v-for="column in overviewColumns" :key="column.key"><span class="attack-status" :class="player.statuses[column.key].tone" :title="player.statuses[column.key].label" :aria-label="column.label+': '+player.statuses[column.key].label" role="img"><i class="attack-dot" aria-hidden="true"></i><span class="attack-detail" aria-hidden="true">{{ player.statuses[column.key].label }}</span><span class="attack-short" aria-hidden="true">{{ player.statuses[column.key].tone==='neutral'?'—':player.statuses[column.key].label.split(' · ')[0] }}</span></span></td>
            </tr>
            <tr v-if="!visibleOverviewRows.length"><td colspan="4" class="empty">{{ loading?'Carregando jogadores…':onlyPending?'Nenhuma pendência identificada nos dados disponíveis.':'Nenhum jogador disponível. Configure o clã e sincronize os dados.' }}</td></tr>
          </tbody>
        </table>
      </section>
      <p v-if="overviewRaidRecord" class="report-note">Raids: lista registrada em {{ new Date(overviewRaidRecord.rosterCapturedAt).toLocaleString('pt-BR') }}{{ overviewRaidRecord.rosterDuringRaid?' durante o fim de semana.':' após o fim de semana; confira a composição histórica do clã.' }} Última consulta em {{ new Date(overviewRaidRecord.syncedAt).toLocaleString('pt-BR') }}.</p>
    </template>
    <template v-else-if="tab==='Raids'">
      <div class="raid-banner"><span class="raid-emblem"><UiIcon name="Raids"/></span><div><small>CAPITAL DO CLÃ</small><h2>Raids da capital</h2><p>Cada ataque conta. Mantenha os registros do seu reino.</p></div></div>
      <div v-if="raidError" class="notice error">{{ raidError }} <span v-if="raidRecords.length">Exibindo registros salvos.</span></div>
      <div class="raid-controls"><label>Fim de semana de raids<select v-model="selectedWeekend" :disabled="loading"><option v-if="!raidRecords.length" value="">Nenhum fim de semana disponível</option><option v-for="r in raidRecords" :key="r.season.startTime" :value="r.season.startTime">{{ dateLabel(r.season.startTime) }} – {{ dateLabel(r.season.endTime) }}</option></select></label><span class="badge" v-if="raid">{{ raidEnded?'CONCLUÍDO':'EM ANDAMENTO · PROVISÓRIO' }}</span><div class="report-actions"><button class="primary" :disabled="!raid||loading" @click="downloadReport('txt')">↓ Relatório semanal</button><button class="secondary" :disabled="!raid||loading" @click="downloadReport('csv')">↓ CSV</button></div></div>
      <template v-if="raid">
        <section class="stats"><article><small>ATACANTES</small><strong>{{ attackingCount }}<em> jogadores</em></strong><span>Inclui atacantes fora da lista registrada</span></article><article><small>{{ raidEnded?'SEM ATAQUES REGISTRADOS':'AINDA NÃO ATACOU' }}</small><strong class="missing-count">{{ nonAttackers.length }}</strong><span>Membros da lista registrada</span></article><article><small>TOTAL DE ATAQUES</small><strong>{{ number(raid.totalAttacks) }}</strong><span>{{ number(raid.enemyDistrictsDestroyed) }} distritos destruídos</span></article><article><small>OURO DA CAPITAL</small><strong>{{ number(raid.capitalTotalLoot) }}</strong><span>{{ number(raid.raidsCompleted) }} raids concluídas</span></article></section>
        <div class="section"><div><small>CHAMADA SEMANAL</small><h2>{{ raidEnded?'Membros sem ataques registrados':'Ainda precisam atacar' }} <em>{{ nonAttackers.length }}</em></h2></div></div>
        <section class="missing-panel"><div class="member-chips"><span v-for="m in nonAttackers" :key="m.tag">{{ m.name }} <small>{{ m.tag }}</small></span><p v-if="!nonAttackers.length">Todos os membros da lista registrada têm ataques registrados.</p></div><p class="report-note">Lista de membros registrada em {{ new Date(raidRecord.rosterCapturedAt).toLocaleString('pt-BR') }}. {{ raidRecord.rosterDuringRaid?'Registrada durante este fim de semana. Mudanças entre sincronizações podem não constar.':'Registrada após este fim de semana. A composição histórica do clã não foi confirmada.' }} A ausência de ataques registrados neste clã não comprova a elegibilidade do jogador nem a participação em outro clã.</p><p v-if="!raidEnded" class="report-note">Este fim de semana ainda está em andamento. O relatório é provisório.</p><small class="sync-note">Última consulta à API: {{ new Date(raidRecord.syncedAt).toLocaleString('pt-BR') }}</small></section>
        <div class="section"><div><small>PARTICIPAÇÃO NAS RAIDS</small><h2>Registro de ataques</h2></div><select v-model="raidFilter" aria-label="Filtrar participação"><option value="all">Todos os jogadores</option><option value="missing">Sem ataques</option><option value="attacked">Atacou</option></select></div>
        <section class="table-wrap"><table><thead><tr><th>JOGADOR</th><th>ATAQUES USADOS</th><th>OURO DA CAPITAL</th><th>SITUAÇÃO</th></tr></thead><tbody><tr v-for="m in visibleRaidPlayers" :key="m.tag"><td><b>{{ m.name }}</b><small>{{ m.tag }}{{ m.inRoster?'':' · Fora da lista registrada' }}</small></td><td>{{ m.attacks }}<span v-if="allowance(m)!=null"> / {{ allowance(m) }}</span></td><td>{{ number(m.capitalResourcesLooted) }}</td><td><span class="participation" :class="{missing:!m.attacks}">{{ raidStatus(m) }}</span></td></tr><tr v-if="!visibleRaidPlayers.length"><td colspan="4" class="empty">Nenhum jogador corresponde a este filtro.</td></tr></tbody></table></section>
      </template>
      <section v-else-if="!loading" class="missing-panel empty">{{ raidError?'Dados de raids indisponíveis. Corrija o problema da API e sincronize novamente.':'Nenhum fim de semana de raids disponível para este clã.' }}</section>
    </template>
    <template v-else-if="tab==='CWL'">
      <div class="raid-banner"><span class="raid-emblem"><UiIcon name="CWL"/></span><div><small>LIGA DE GUERRA</small><h2>Liga de Guerra de Clãs</h2><p>O grupo da liga e as escalações diárias do seu clã.</p></div></div>
      <p v-if="leagueLoading" class="loading">Carregando grupo da liga…</p>
      <div v-if="leagueError" class="notice error">{{ leagueError }}</div>
      <section v-if="!leagueGroup&&!leagueLoading&&!loading" class="missing-panel empty">{{ leagueMessage||leagueError||'Configure um clã e o token da API para carregar a Liga de Guerra de Clãs.' }}</section>
      <template v-if="leagueGroup">
        <section class="stats three cwl-stats"><article><small>TEMPORADA</small><strong>{{ leagueGroup.season||'—' }}</strong><span>{{ clan?.warLeague?.name||'Grupo atual da liga' }}</span></article><article><small>SITUAÇÃO DA LIGA</small><strong class="small-strong">{{ stateLabel(leagueGroup.state) }}</strong><span>{{ leagueGroup.clans?.length||0 }} clãs neste grupo</span></article><article><small>RODADAS</small><strong>{{ leagueGroup.rounds?.length||0 }}</strong><span>Selecione uma rodada para acompanhar a escalação</span></article></section>
        <div class="section"><div><small>BATALHAS DIÁRIAS</small><h2>Rodadas da liga</h2></div></div>
        <div class="league-rounds"><button class="secondary" :class="{selected:leagueView==='all'}" :disabled="allRoundsLoading||loading||leagueLoading" @click="loadAllRounds"><b>Todos</b><small>Temporada completa</small></button><button v-for="(round,index) in leagueGroup.rounds||[]" :key="index" class="secondary" :class="{selected:leagueView==='round'&&selectedRound===index+1}" :disabled="leagueLoading||loading" @click="chooseRound(index+1)"><b>Rodada {{ index+1 }}</b><small>{{ roundAssigned(round)?'Ver batalha':'Ainda não definida' }}</small></button></div>
        <template v-if="leagueView==='all'">
          <div class="section"><div><small>TEMPORADA COMPLETA · SOMENTE ESTE CLÃ</small><h2>Todos os jogadores <em>{{ seasonPlayers.length }}</em></h2><p>Inscritos na liga e escalados nas batalhas. Cada coluna mostra uma rodada.</p></div><button class="secondary" :disabled="allRoundsLoading||!allRounds.length" @click="downloadBattle('CWL')">↓ Exportar CSV · Todos</button></div>
          <p v-if="allRoundsLoading" class="loading">Carregando as rodadas disponíveis…</p>
          <p v-for="entry in allRounds.filter(entry=>entry.error)" :key="entry.round" class="notice error">Rodada {{ entry.round }}: {{ entry.error }}. O relatório indica a ausência desses dados.</p>
          <section v-if="!allRoundsLoading" class="table-wrap"><table><thead><tr><th>JOGADOR</th><th v-for="entry in allRounds" :key="entry.round">RODADA {{ entry.round }}</th></tr></thead><tbody><tr v-for="player in seasonPlayers" :key="player.tag"><td><b>{{ player.name }}</b><small>{{ player.tag }}</small></td><td v-for="(status,index) in player.rounds" :key="index"><span class="attack-status" :class="status.tone"><i class="attack-dot" aria-hidden="true"></i>{{ status.used==null?'':status.used+'/'+status.limit+' · ' }}{{ status.label }}</span></td></tr><tr v-if="!seasonPlayers.length"><td :colspan="allRounds.length+1" class="empty">Nenhum jogador disponível nesta temporada.</td></tr></tbody></table></section>
        </template>
        <template v-else>
        <p v-if="leagueWarLoading" class="loading">Carregando rodada {{ selectedRound }}…</p><div v-if="roundError" class="notice error">{{ roundError }} <button @click="loadLeagueRound">↻</button></div><div v-if="roundMessage&&!leagueWarLoading" class="notice">{{ roundMessage }}</div>
        <template v-if="leagueWar">
          <section class="war cwl-battle"><div class="war-head"><div><span class="badge">{{ stateLabel(leagueWar.state).toUpperCase() }} · RODADA {{ selectedRound }}</span><h3>{{ leagueOurSide?.name }} vs {{ leagueEnemySide?.name }}</h3><p>{{ leagueWar.teamSize }} vs {{ leagueWar.teamSize }} · {{ leagueWar.attacksPerMember||1 }} ataque por jogador</p></div><div class="score"><b>{{ leagueOurSide?.stars??0 }}</b> : <b>{{ leagueEnemySide?.stars??0 }}</b><small>ESTRELAS DE GUERRA</small></div></div><div class="war-foot"><span>{{ leagueOurSide?.attacks||0 }} / {{ (leagueWar.teamSize||0)*(leagueWar.attacksPerMember||1) }} ataques usados</span><span>{{ leagueOurSide?.destructionPercentage??0 }}% de destruição</span><span v-if="leagueWar.endTime">Termina em {{ raidDate(leagueWar.endTime).toLocaleString('pt-BR') }}</span></div></section>
          <div class="section"><div><small>RODADA {{ selectedRound }} · ESCALAÇÃO SELECIONADA</small><h2>{{ leagueWar.state==='warEnded'?'Ataques não realizados':'Jogadores que ainda não atacaram' }} <em>{{ leagueMissing.length }}</em></h2></div></div>
          <section class="missing-panel"><div class="member-chips"><span v-for="m in leagueMissing" :key="m.tag">{{ m.name }}<small>{{ m.tag }}</small></span><p v-if="!leagueMissing.length">Todos os jogadores escalados nesta rodada já atacaram.</p></div><p class="report-note">Apenas jogadores escalados nesta rodada são contabilizados. {{ leagueWar.state==='preparation'?'A preparação ainda está em andamento; os ataques não começaram.':leagueWar.state==='warEnded'?'Esta batalha terminou.':'A batalha ainda está em andamento.' }}</p></section>
          <div class="section"><div><small>ACOMPANHAMENTO DE ATAQUES DA LIGA</small><h2>Rodada {{ selectedRound }} — todos os escalados</h2></div><button class="secondary" :disabled="leagueWarLoading||!leagueMembers.length" @click="downloadBattle('CWL')">↓ Exportar CSV</button></div>
          <section class="table-wrap"><table><thead><tr><th>JOGADOR</th><th>CENTRO DE VILA</th><th>ATAQUES</th><th>ESTRELAS</th><th>DESTRUIÇÃO</th><th>ALVO</th></tr></thead><tbody><tr v-for="m in leagueMembers" :key="m.tag"><td><b>{{ m.mapPosition }} · {{ m.name }}</b><small>{{ m.tag }}</small></td><td>{{ m.townhallLevel??m.townHallLevel??'—' }}</td><td>{{ m.attacks?.length||0 }} / {{ leagueWar.attacksPerMember||1 }}</td><td class="stars">{{ stars(m) }} ★</td><td>{{ percent(m) }}%</td><td>{{ m.attacks?.map(a=>a.defenderTag).join(', ')||'Nenhum ataque registrado' }}</td></tr><tr v-if="!leagueMembers.length"><td colspan="6" class="empty">Nenhuma escalação disponível para esta rodada.</td></tr></tbody></table></section>
        </template>
        </template>
        <div class="section"><div><small>O GRUPO DA LIGA</small><h2>Clãs inscritos</h2></div></div>
        <section class="table-wrap"><table><thead><tr><th>CLÃ</th><th>NÍVEL</th><th>JOGADORES INSCRITOS</th></tr></thead><tbody><tr v-for="c in leagueGroup.clans||[]" :key="c.tag" :class="{'our-league-clan':c.tag?.replace(/^#/,'')===activeClanTag}"><td><b>{{ c.name }}{{ c.tag?.replace(/^#/,'')===activeClanTag?' · Seu clã':'' }}</b><small>{{ c.tag }}</small></td><td>{{ c.clanLevel??'—' }}</td><td>{{ c.members?.length??'—' }}</td></tr></tbody></table></section>
      </template>
    </template>
    <template v-else-if="tab==='War'"><div v-if="warError" class="notice">{{ warError }}</div><div class="stats three"><article><small>SUAS ESTRELAS</small><strong>{{ war?.clan?.stars??'—' }}</strong><span>{{ war?.clan?.attacks||0 }} ataques usados</span></article><article><small>ESTRELAS DO ADVERSÁRIO</small><strong>{{ war?.opponent?.stars??'—' }}</strong><span>{{ war?.opponent?.attacks||0 }} ataques usados</span></article><article><small>SITUAÇÃO DA GUERRA</small><strong class="small-strong">{{ warState }}</strong><span>{{ war?.teamSize||0 }} participantes</span></article></div><div class="section"><div><small>ACOMPANHAMENTO DE ATAQUES</small><h2>Ataques de guerra</h2></div><button class="secondary" :disabled="loading||!!warError||!warMembers.length" @click="downloadBattle('Guerra')">↓ Exportar CSV</button></div><section class="table-wrap"><table><thead><tr><th>JOGADOR</th><th>ATAQUES</th><th>ESTRELAS</th><th>DESTRUIÇÃO</th><th>ALVO</th></tr></thead><tbody><tr v-for="m in warMembers" :key="m.tag"><td><b>{{ m.name }}</b><small>{{ m.tag }}</small></td><td>{{ m.attacks?.length||0 }} / {{ war?.attacksPerMember||2 }}</td><td class="stars">{{ '★'.repeat(stars(m)) }}</td><td>{{ percent(m) }}%</td><td>{{ m.attacks?.map(a=>a.defenderTag).join(', ')||'Não atacou' }}</td></tr><tr v-if="!warMembers.length"><td class="empty" colspan="5">Nenhum ataque de guerra ativo para exibir.</td></tr></tbody></table></section></template>
    <template v-else-if="tab==='Roster'"><div class="section"><div><small>TODOS OS MEMBROS</small><h2>Membros do clã <em>{{ members.length }}</em></h2></div></div><section class="table-wrap"><table><thead><tr><th>JOGADOR</th><th>CARGO</th><th>NÍVEL</th><th>TROFÉUS</th><th>DOAÇÕES</th><th>ESTRELAS DE GUERRA</th></tr></thead><tbody><tr v-for="m in members" :key="m.tag"><td><b>{{ m.name }}</b><small>{{ m.tag }}</small></td><td>{{ roleLabel(m.role) }}</td><td>{{ m.expLevel }}</td><td>🏆 {{ m.trophies }}</td><td>{{ m.donations }}</td><td>{{ m.warStars }}</td></tr><tr v-if="!members.length"><td colspan="6" class="empty">Os membros aparecerão aqui quando a API estiver conectada.</td></tr></tbody></table></section></template>
    <template v-else-if="tab==='Contacts'"><ContactsPanel :configured="config.contactsConfigured" :loading="loading" :clans="config.clans"/></template>
    <KeepAlive><LeadershipPanel v-if="tab==='Leadership'"/></KeepAlive>
    <PendingMessage :war-mode="tab==='War'" v-if="['Overview','Raids','CWL','War'].includes(tab)" :clan-name="clan?.name||('Clã '+selectedClan)" :clan-tag="activeClanTag" :items="messageItems" :busy="messageBusy" :context="messageContext"/>
    <footer>IMPÉRIO BÁRBARO CRM &nbsp;·&nbsp; CONTROLE DE ATAQUES &nbsp;·&nbsp; {{ new Date().getFullYear() }}</footer>
  </div></main><nav class="mobile-nav"><button v-for="item in tabs" :key="item" :class="{active:tab===item}" @click="tab=item"><UiIcon :name="item"/><small>{{ tabLabels[item] }}</small></button></nav>
</div>
</template>
