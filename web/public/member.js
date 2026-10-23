import { memberSetup, memberClan, memberAppURL } from './pwa-state.js'
const byId=id=>document.getElementById(id)
const ios=/iPad|iPhone|iPod/.test(navigator.userAgent)||(navigator.platform==='MacIntel'&&navigator.maxTouchPoints>1)
const installed=()=>matchMedia('(display-mode: standalone)').matches||navigator.standalone===true
const supported=()=>isSecureContext&&'serviceWorker'in navigator&&'PushManager'in window&&'Notification'in window
let deferredInstall,registration,config,subscribed=false,busy=false,feedGeneration=0,setupGeneration=0,feedTimer,checking=true,bootPromise
const permission=()=>('Notification'in window?Notification.permission:'default')
window.addEventListener('beforeinstallprompt',event=>{event.preventDefault();deferredInstall=event;render()})
window.addEventListener('appinstalled',()=>{deferredInstall=null;resumeSetup()})
try{byId('member-clan').value=memberClan(localStorage.getItem('imperio-clan'))}catch{}
async function request(path,options={}){
 const response=await fetch(path,{credentials:'omit',cache:'no-store',...options,headers:{'Content-Type':'application/json','X-Clanboard-Request':'1'}})
 const result=await response.json().catch(()=>({}))
 if(!response.ok)throw Error(result.error||'Não foi possível conectar. Verifique sua internet e tente novamente.')
 return result
}
function step(text){const li=document.createElement('li');li.textContent=text;return li}
function render(){
 const clan=memberClan(byId('member-clan').value),state=memberSetup({ios,installed:installed(),supported:supported(),permission:permission(),subscribed:subscribed&&!!clan})
 document.body.classList.toggle('is-ready',state==='ready')
 for(const id of ['enable-notifications','install-app','install-instructions','retry-setup','test-notification','open-app'])byId(id).hidden=true
 byId('setup-status').className='setup-status'
 byId('member-clan').disabled=busy||checking
 const feedReady=document.body.dataset.page==='member'&&permission()==='granted'&&subscribed&&!!clan
 byId('member-feed').hidden=!feedReady
 if(!feedReady){feedGeneration++;byId('feed-items').replaceChildren()}
 if(checking){byId('setup-status').textContent='Verificando os avisos deste celular…';byId('setup-description').textContent='Conferindo a permissão e a inscrição do seu clã.';return}
 if(!config){byId('setup-status').textContent='Conecte-se para preparar o app.';byId('setup-description').textContent='Precisamos de internet para ativar os avisos do seu clã.';byId('retry-setup').hidden=false;return}
 if(state==='install-first'){
  byId('setup-status').textContent='No iPhone, instale primeiro.'
  byId('setup-description').textContent='Depois, abra pelo ícone do Império e ative todos os avisos para concluir.'
  byId('install-instructions').hidden=false
  byId('install-steps').replaceChildren(...['Abra este link no Safari.','Toque em Compartilhar e em Adicionar à Tela de Início.','Toque em Adicionar, abra o Império pelo ícone e ative os avisos.'].map(step))
 }else if(state==='unsupported'){
  byId('setup-status').textContent='Abra no navegador do celular.';byId('setup-status').classList.add('warning')
  byId('setup-description').textContent='Se estiver no WhatsApp, abra este link no Chrome (Android) ou Safari (iPhone). É preciso um navegador com suporte a notificações e conexão segura.'
  byId('retry-setup').hidden=false
 }else if(state==='blocked'){
  byId('setup-status').textContent='Seus avisos estão bloqueados.';byId('setup-status').classList.add('warning')
  byId('setup-description').textContent='Permita as notificações nas configurações deste site ou app no celular. Depois, volte aqui e toque em Verificar novamente.';byId('retry-setup').hidden=false
 }else if(state==='enable'){
  byId('setup-status').textContent=clan?'Ative todos os avisos do clã.':'Primeiro, escolha seu clã.'
  byId('setup-description').textContent='Permita as notificações de guerra, raids, jogos, CWL e comunicados. Você precisa ativá-las para concluir a preparação do app.'
  byId('enable-notifications').hidden=false;byId('enable-notifications').disabled=busy||!clan||!registration
  byId('enable-notifications').textContent=busy?'Ativando avisos…':'Ativar todos os avisos'
 }else{
  byId('setup-status').classList.add('ready');byId('setup-status').textContent='Todos os avisos estão ativados.'
  byId('test-notification').hidden=false
  if(state==='install'){
   byId('setup-description').textContent='Agora instale o Império para acessar pelo ícone na tela inicial.'
   if(deferredInstall){byId('install-app').hidden=false;byId('install-app').disabled=busy}
   else{byId('install-instructions').hidden=false;byId('install-steps').replaceChildren(...['Abra o menu do navegador.','Escolha Instalar app ou Adicionar à tela inicial.','Se a opção ainda não aparecer, permaneça nesta página por alguns instantes e tente novamente.'].map(step))}
  }else{
   byId('setup-description').textContent='Seu celular está pronto para receber os comunicados do clã.'
   if(document.body.dataset.page==='install'){byId('open-app').hidden=false;byId('open-app').href=memberAppURL(clan)}
  }
 }

}
function appKey(){return Uint8Array.from(atob(config.publicKey.replace(/-/g,'+').replace(/_/g,'/')),c=>c.charCodeAt(0))}
async function saveSubscription(subscription){await request('/api/push/subscription',{method:'POST',body:JSON.stringify({clanId:memberClan(byId('member-clan').value),subscription:subscription.toJSON()})})}
async function sync(){
 if(busy)return
 const current=++setupGeneration
 if(!config||!registration||!supported()||permission()!=='granted'||!byId('member-clan').value){subscribed=false;checking=false;render();return}
 checking=true;busy=true;render()
 try{const subscription=await registration.pushManager.getSubscription();subscribed=false;if(subscription){await saveSubscription(subscription);if(current===setupGeneration)subscribed=true}}
 catch(e){subscribed=false;if(current===setupGeneration)byId('action-feedback').textContent=e.message}
 finally{busy=false;checking=false}
 if(current!==setupGeneration)return
 render();if(subscribed)loadFeed()
}
byId('enable-notifications').addEventListener('click',async()=>{
 if(busy||!registration||!config||!byId('member-clan').value)return
 busy=true;byId('action-feedback').textContent='';render()
 try{
  // Ask directly from the tap, before any network call (required on iOS).
  const result=await Notification.requestPermission()
  if(result!=='granted')throw Error('Ative as notificações para receber os avisos e concluir a preparação.')
  let subscription=await registration.pushManager.getSubscription()
  if(!subscription)subscription=await registration.pushManager.subscribe({userVisibleOnly:true,applicationServerKey:appKey()})
  await saveSubscription(subscription);subscribed=true
 }catch(e){byId('action-feedback').textContent=e.message||'Não foi possível ativar. Abra no navegador do celular e tente novamente.'}
 finally{busy=false;render();if(subscribed)loadFeed()}
})
byId('install-app').addEventListener('click',async()=>{
 if(!subscribed||permission()!=='granted'){render();return}
 if(!deferredInstall)return
 const prompt=deferredInstall;deferredInstall=null
 try{await prompt.prompt();const choice=await prompt.userChoice;byId('action-feedback').textContent=choice.outcome==='accepted'?'Instalação solicitada. Procure o ícone do Império na tela inicial.':'Você pode instalar quando quiser. Os avisos precisam continuar ativados.'}catch{byId('action-feedback').textContent='Use a opção Instalar app no menu do navegador.'}
 render()
})
byId('member-clan').addEventListener('change',()=>{try{localStorage.setItem('imperio-clan',memberClan(byId('member-clan').value))}catch{}byId('action-feedback').textContent='';sync()})
byId('retry-setup').addEventListener('click',()=>boot())
byId('test-notification').addEventListener('click',async()=>{
 const button=byId('test-notification');button.disabled=true;byId('action-feedback').textContent='Enviando um teste só para este celular…'
 try{
  const subscription=await registration.pushManager.getSubscription()
  if(!subscription)throw Error('Verifique novamente a ativação dos avisos antes de testar.')
  await request('/api/push/test',{method:'POST',body:JSON.stringify({subscription:subscription.toJSON()})})
  byId('action-feedback').textContent='Teste aceito pelo serviço de push. Confira a barra de notificações. Se não aparecer, confira os avisos do app e do navegador nas configurações do celular.'
 }catch(e){byId('action-feedback').textContent=e.message}
 finally{button.disabled=false}
})
async function loadFeed(){
 if(document.body.dataset.page!=='member'||!subscribed||permission()!=='granted')return
 const generation=++feedGeneration,clan=memberClan(byId('member-clan').value)
 byId('feed-status').textContent='Buscando avisos…'
 try{
  const data=await request('/api/push/announcements?clan='+clan)
  if(generation!==feedGeneration)return
  const cards=data.items.map(item=>{const card=document.createElement('article');card.className='feed-card';card.id=item.id;const time=document.createElement('time');time.dateTime=new Date(item.createdAt*1000).toISOString();time.textContent=new Date(item.createdAt*1000).toLocaleString('pt-BR');const title=document.createElement('h3');title.textContent=item.title;const p=document.createElement('p');p.textContent=item.body;card.append(time,title,p);return card})
  byId('feed-items').replaceChildren(...cards);byId('feed-status').textContent=cards.length?'':'Os próximos avisos do seu clã aparecerão aqui.'
  const linked=document.getElementById(location.hash.slice(1));if(linked?.classList.contains('feed-card')){linked.classList.add('highlighted');linked.scrollIntoView({block:'center',behavior:'instant'})}
 }catch(e){if(generation===feedGeneration)byId('feed-status').textContent='Você está sem acesso aos avisos agora. Verifique a internet e toque em Atualizar.'}
}
byId('refresh-feed').addEventListener('click',loadFeed)
function resumeSetup(){if(config&&registration&&!bootPromise&&!document.hidden)sync()}
document.addEventListener('visibilitychange',resumeSetup)
window.addEventListener('online',()=>boot())
window.addEventListener('focus',resumeSetup)
if('serviceWorker'in navigator)navigator.serviceWorker.addEventListener('controllerchange',resumeSetup)
if(navigator.permissions)navigator.permissions.query({name:'notifications'}).then(status=>status.addEventListener('change',resumeSetup)).catch(()=>{})
window.addEventListener('hashchange',loadFeed)
function boot(){
 if(bootPromise)return bootPromise
 bootPromise=prepare().finally(()=>{bootPromise=null})
 return bootPromise
}
async function prepare(){
 checking=true;render()
 try{
  config=await request('/api/push/config')
  if('serviceWorker'in navigator){await navigator.serviceWorker.register('/pwa-sw.js',{updateViaCache:'none'});registration=await navigator.serviceWorker.ready}
  // A notification link selects its clan without trusting arbitrary values.
  const linked=memberClan(new URLSearchParams(location.search).get('clan'));if(linked){byId('member-clan').value=linked;history.replaceState(null,'',location.pathname+location.hash);try{localStorage.setItem('imperio-clan',linked)}catch{}}
  await sync()
 }catch(e){checking=false;byId('action-feedback').textContent=e.message;render()}
}
render();boot();feedTimer=setInterval(()=>{if(!document.hidden)loadFeed()},60000)
