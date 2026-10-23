import { memberSetup } from './pwa-state.js'
const $=id=>document.getElementById(id)
const ios=/iPad|iPhone|iPod/.test(navigator.userAgent)||(navigator.platform==='MacIntel'&&navigator.maxTouchPoints>1)
const installed=()=>matchMedia('(display-mode: standalone)').matches||navigator.standalone===true
const supported=()=>isSecureContext&&'serviceWorker'in navigator&&'PushManager'in window&&'Notification'in window
let deferredInstall,registration,config,subscribed=false,busy=false,feedGeneration=0,setupGeneration=0,feedTimer
const permission=()=>('Notification'in window?Notification.permission:'default')
window.addEventListener('beforeinstallprompt',event=>{event.preventDefault();deferredInstall=event;render()})
window.addEventListener('appinstalled',()=>{deferredInstall=null;render()})
try{$('member-clan').value=localStorage.getItem('imperio-clan')||''}catch{}
async function request(path,options={}){
 const response=await fetch(path,{credentials:'omit',cache:'no-store',...options,headers:{'Content-Type':'application/json','X-Clanboard-Request':'1'}})
 const result=await response.json().catch(()=>({}))
 if(!response.ok)throw Error(result.error||'Não foi possível conectar. Verifique sua internet e tente novamente.')
 return result
}
function step(text){const li=document.createElement('li');li.textContent=text;return li}
function render(){
 const clan=$('member-clan').value,state=memberSetup({ios,installed:installed(),supported:supported(),permission:permission(),subscribed:subscribed&&!!clan})
 document.body.classList.toggle('is-ready',state==='ready')
 for(const id of ['enable-notifications','install-app','install-instructions','retry-setup','test-notification','open-app'])$(id).hidden=true
 $('setup-status').className='setup-status'
 $('member-clan').disabled=busy
 if(!config){$('setup-status').textContent='Conecte-se para preparar o app.';$('setup-description').textContent='Precisamos de internet para ativar os avisos do seu clã.';$('retry-setup').hidden=false;return}
 if(state==='install-first'){
  $('setup-status').textContent='No iPhone, instale primeiro.'
  $('setup-description').textContent='Depois, abra pelo ícone do Império e ative todos os avisos para concluir.'
  $('install-instructions').hidden=false
  $('install-steps').replaceChildren(...['Abra este link no Safari.','Toque em Compartilhar e em Adicionar à Tela de Início.','Toque em Adicionar, abra o Império pelo ícone e ative os avisos.'].map(step))
 }else if(state==='unsupported'){
  $('setup-status').textContent='Abra no navegador do celular.';$('setup-status').classList.add('warning')
  $('setup-description').textContent='Se estiver no WhatsApp, abra este link no Chrome (Android) ou Safari (iPhone). É preciso um navegador com suporte a notificações e conexão segura.'
  $('retry-setup').hidden=false
 }else if(state==='blocked'){
  $('setup-status').textContent='Seus avisos estão bloqueados.';$('setup-status').classList.add('warning')
  $('setup-description').textContent='Permita as notificações nas configurações deste site ou app no celular. Depois, volte aqui e toque em Verificar novamente.';$('retry-setup').hidden=false
 }else if(state==='enable'){
  $('setup-status').textContent=clan?'Ative todos os avisos do clã.':'Primeiro, escolha seu clã.'
  $('setup-description').textContent='Permita as notificações de guerra, raids, jogos, CWL e comunicados. Você precisa ativá-las para concluir a preparação do app.'
  $('enable-notifications').hidden=false;$('enable-notifications').disabled=busy||!clan||!registration
  $('enable-notifications').textContent=busy?'Ativando avisos…':'Ativar todos os avisos'
 }else{
  $('setup-status').classList.add('ready');$('setup-status').textContent='Todos os avisos estão ativados.'
  $('test-notification').hidden=false
  if(state==='install'){
   $('setup-description').textContent='Agora instale o Império para acessar pelo ícone na tela inicial.'
   if(deferredInstall){$('install-app').hidden=false;$('install-app').disabled=busy}
   else{$('install-instructions').hidden=false;$('install-steps').replaceChildren(...['Abra o menu do navegador.','Escolha Instalar app ou Adicionar à tela inicial.','Se a opção ainda não aparecer, permaneça nesta página por alguns instantes e tente novamente.'].map(step))}
  }else{
   $('setup-description').textContent='Seu celular está pronto para receber os comunicados do clã.'
   if(document.body.dataset.page==='install'){$('open-app').hidden=false;$('open-app').href='/app?clan='+clan}
  }
 }
 const feedReady=document.body.dataset.page==='member'&&permission()==='granted'&&subscribed&&!!clan
 $('member-feed').hidden=!feedReady
 if(!feedReady){feedGeneration++;$('feed-items').replaceChildren()}
}
function appKey(){return Uint8Array.from(atob(config.publicKey.replace(/-/g,'+').replace(/_/g,'/')),c=>c.charCodeAt(0))}
async function saveSubscription(subscription){await request('/api/push/subscription',{method:'POST',body:JSON.stringify({clanId:$('member-clan').value,subscription:subscription.toJSON()})})}
async function sync(){
 if(busy)return
 const current=++setupGeneration
 subscribed=false
 if(!config||!supported()||permission()!=='granted'||!$('member-clan').value){render();return}
 busy=true;render()
 try{const subscription=await registration?.pushManager.getSubscription();if(subscription){await saveSubscription(subscription);if(current===setupGeneration)subscribed=true}}
 catch(e){if(current===setupGeneration)$('action-feedback').textContent=e.message}
 finally{busy=false}
 if(current!==setupGeneration)return
 render();if(subscribed)loadFeed()
}
$('enable-notifications').addEventListener('click',async()=>{
 if(busy||!registration||!config||!$('member-clan').value)return
 busy=true;$('action-feedback').textContent='';render()
 try{
  // Ask directly from the tap, before any network call (required on iOS).
  const result=await Notification.requestPermission()
  if(result!=='granted')throw Error('Ative as notificações para receber os avisos e concluir a preparação.')
  let subscription=await registration.pushManager.getSubscription()
  if(!subscription)subscription=await registration.pushManager.subscribe({userVisibleOnly:true,applicationServerKey:appKey()})
  await saveSubscription(subscription);subscribed=true
 }catch(e){$('action-feedback').textContent=e.message||'Não foi possível ativar. Abra no navegador do celular e tente novamente.'}
 finally{busy=false;render();if(subscribed)loadFeed()}
})
$('install-app').addEventListener('click',async()=>{
 if(!subscribed||permission()!=='granted'){render();return}
 if(!deferredInstall)return
 const prompt=deferredInstall;deferredInstall=null
 try{await prompt.prompt();const choice=await prompt.userChoice;$('action-feedback').textContent=choice.outcome==='accepted'?'Instalação solicitada. Procure o ícone do Império na tela inicial.':'Você pode instalar quando quiser. Os avisos precisam continuar ativados.'}catch{$('action-feedback').textContent='Use a opção Instalar app no menu do navegador.'}
 render()
})
$('member-clan').addEventListener('change',()=>{try{localStorage.setItem('imperio-clan',$('member-clan').value)}catch{}$('action-feedback').textContent='';sync()})
$('retry-setup').addEventListener('click',()=>boot())
$('test-notification').addEventListener('click',async()=>{try{await registration.showNotification('🛡️ Império Bárbaro',{body:'Tudo pronto! Este é um teste de aviso neste celular.',icon:'/app-icon-192.png',tag:'imperio-test',silent:false,vibrate:[220,100,220],data:{url:'/app'}});$('action-feedback').textContent='Teste exibido neste celular. Confira som, banners e tela bloqueada nas configurações.'}catch{$('action-feedback').textContent='O teste não pôde ser exibido. Verifique a permissão de notificações.'}})
async function loadFeed(){
 if(document.body.dataset.page!=='member'||!subscribed||permission()!=='granted')return
 const generation=++feedGeneration,clan=$('member-clan').value
 $('feed-status').textContent='Buscando avisos…'
 try{
  const data=await request('/api/push/announcements?clan='+clan)
  if(generation!==feedGeneration)return
  const cards=data.items.map(item=>{const card=document.createElement('article');card.className='feed-card';card.id=item.id;const time=document.createElement('time');time.dateTime=new Date(item.createdAt*1000).toISOString();time.textContent=new Date(item.createdAt*1000).toLocaleString('pt-BR');const title=document.createElement('h3');title.textContent=item.title;const p=document.createElement('p');p.textContent=item.body;card.append(time,title,p);return card})
  $('feed-items').replaceChildren(...cards);$('feed-status').textContent=cards.length?'':'Os próximos avisos do seu clã aparecerão aqui.'
  const linked=document.getElementById(location.hash.slice(1));if(linked?.classList.contains('feed-card')){linked.classList.add('highlighted');linked.scrollIntoView({block:'center',behavior:'instant'})}
 }catch(e){if(generation===feedGeneration)$('feed-status').textContent='Você está sem acesso aos avisos agora. Verifique a internet e toque em Atualizar.'}
}
$('refresh-feed').addEventListener('click',loadFeed)
document.addEventListener('visibilitychange',()=>{if(!document.hidden)sync()})
window.addEventListener('online',()=>boot())
window.addEventListener('hashchange',loadFeed)
async function boot(){
 try{
  config=await request('/api/push/config')
  if('serviceWorker'in navigator){registration=await navigator.serviceWorker.register('/pwa-sw.js');await navigator.serviceWorker.ready}
  // A notification link selects its clan without trusting arbitrary values.
  const linked=new URLSearchParams(location.search).get('clan');if(['1','2'].includes(linked)){$('member-clan').value=linked;history.replaceState(null,'',location.pathname+location.hash);try{localStorage.setItem('imperio-clan',linked)}catch{}}
  await sync()
 }catch(e){$('action-feedback').textContent=e.message;render()}
}
render();boot();feedTimer=setInterval(()=>{if(!document.hidden)loadFeed()},60000)
