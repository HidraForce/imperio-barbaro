import test from 'node:test'
import assert from 'node:assert/strict'
import { Readable, Writable } from 'node:stream'
import { mkdtemp, writeFile, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { createGateway, bindingURL } from './gateway.mjs'
test('Member PWA and push endpoints stay public without exposing dashboard credentials',async()=>{
 const dir=await mkdtemp(join(tmpdir(),'pwa-gateway-'))
 await writeFile(join(dir,'install.html'),'<html>Instalar Império</html>')
 await writeFile(join(dir,'member.webmanifest'),'{}')
 const calls=[]
 const handler=createGateway({dist:dir,apiBase:'https://api.internal',fetcher:async(url,options)=>{calls.push({url:String(url),options});return new Response('{"subscribed":true}',{headers:{'content-type':'application/json'}})}})
 try{
  assert.equal((await invoke(handler,'/instalar',{host:'imperiobarbaro.com.br'})).code,200)
  const manifest=await invoke(handler,'/member.webmanifest',{host:'imperiobarbaro.com.br'});assert.equal(manifest.code,200);assert.equal(manifest.headers['content-type'],'application/manifest+json; charset=utf-8')
  const headers={host:'imperiobarbaro.com.br',authorization:'Basic private','x-contacts-session':'secret','content-type':'application/json','x-clanboard-request':'1'}
  assert.equal((await invoke(handler,'/api/push/subscription',headers,'POST','{}')).code,200)
  assert.equal(calls.at(-1).options.headers.get('authorization'),null)
  assert.equal(calls.at(-1).options.headers.get('x-contacts-session'),null)
  assert.equal((await invoke(handler,'/api/push/subscription',{...headers,origin:'https://evil.test'},'POST','{}')).code,403)
  assert.equal((await invoke(handler,'/api/announcements',{host:'imperiobarbaro.com.br'},'POST','{}')).code,405)
 }finally{await rm(dir,{recursive:true})}
})
class Reply extends Writable {
 constructor(){super();this.headersSent=false;this.data='';this.chunks=[];this.code=0;this.headers={}}
 writeHead(code,headers){this.code=code;this.headers=headers;this.headersSent=true}
 _write(chunk,encoding,done){this.chunks.push(Buffer.from(chunk));this.data+=chunk.toString();done()}
}
async function invoke(handler,path,headers={},method='GET',body=''){
 const req=Readable.from(body?[body]:[]);Object.assign(req,{url:path,method,headers:{host:'crm.test',...headers}})
 const res=new Reply();await handler(req,res);return res
}
test('Interface e API são autenticadas pelo serviço interno; URL só vem do binding',async()=>{
 const dir=await mkdtemp(join(tmpdir(),'crm-gateway-'));await writeFile(join(dir,'index.html'),'<html>CRM</html>')
 try{
  const calls=[]
  const handler=createGateway({dist:dir,apiBase:'https://api.internal/base',fetcher:async(url,options)=>{
   calls.push({url:String(url),options})
   if(!options.headers.get('authorization'))return new Response('{"error":"Login necessário"}',{status:401,headers:{'www-authenticate':'Basic realm="CRM"'}})
   return new Response('{}',{headers:{'content-security-policy':"default-src 'self'"}})
  }})
  const denied=await invoke(handler,'/');assert.equal(denied.code,401);assert.ok(denied.headers['www-authenticate'])
  const allowed=await invoke(handler,'/',{authorization:'Basic teste'});assert.equal(allowed.code,200);assert.equal(allowed.data,'<html>CRM</html>');assert.equal(allowed.headers['cache-control'],'no-store')
  assert.equal(calls[0].url,'https://api.internal/base/api/health')
  const api=await invoke(handler,'/api/war?clan=2',{authorization:'Basic teste',origin:'https://crm.test','sec-fetch-site':'same-origin'})
  assert.equal(api.code,200);assert.equal(calls.at(-1).url,'https://api.internal/base/api/war?clan=2');assert.equal(calls.at(-1).options.headers.get('origin'),null)
 }finally{await rm(dir,{recursive:true})}
})
test('Origem externa e gravação sem proteção são barradas antes de chamar a API',async()=>{
 let calls=0;const handler=createGateway({apiBase:'https://api.internal',fetcher:async()=>{calls++;return new Response('{}')}})
 assert.equal((await invoke(handler,'/api/contacts',{origin:'https://outro.test'})).code,403)
 assert.equal((await invoke(handler,'/api/contacts',{},'POST','{}')).code,403)
 assert.equal((await invoke(handler,'/api/contacts',{'sec-fetch-site':'same-site'})).code,403);assert.equal(calls,0)
})
test('POST mantém método, token de sessão e corpo sem encaminhar headers forjados',async()=>{
 let captured
 const handler=createGateway({apiBase:'https://api.internal',fetcher:async(url,options)=>{
  let body='';for await(const chunk of options.body)body+=chunk;captured={options,body};return new Response(null,{status:204})
 }})
 const res=await invoke(handler,'/api/contacts',{origin:'https://crm.test',authorization:'Basic exemplo','x-clanboard-request':'1','x-contacts-session':'token','content-type':'application/json','x-forwarded-host':'malicioso.test'},'POST','{"accounts":[]}')
 assert.equal(res.code,204);assert.equal(captured.options.method,'POST');assert.equal(captured.body,'{"accounts":[]}');assert.equal(captured.options.headers.get('x-contacts-session'),'token');assert.equal(captured.options.headers.get('x-forwarded-host'),null)
})
test('Falha na API não entrega a página sem login nem revela endereços internos',async()=>{
 const handler=createGateway({apiBase:'https://api.internal',fetcher:async()=>{throw Error('segredo')}})
 const response=await invoke(handler,'/');assert.equal(response.code,502);assert.ok(!response.data.includes('segredo'));assert.ok(!response.data.includes('api.internal'))
 for(const url of ['ftp://interno','https://user:pass@interno','https://interno/?token=valor'])assert.throws(()=>bindingURL(url))
})

test('Domínio principal carrega regras atuais sem encaminhar credenciais',async()=>{
 const dir=await mkdtemp(join(tmpdir(),'home-gateway-'))
 await writeFile(join(dir,'home.html'),'<html>Império Bárbaro</html>')
 await writeFile(join(dir,'home.css'),'body{}')
 let calls=0
 const handler=createGateway({dist:dir,apiBase:'https://api.internal',fetcher:async(url,options)=>{calls++;if(String(url)==='https://api.internal/public/home'){assert.equal(options.headers,undefined);return new Response('<html>Império Bárbaro</html>')};return new Response('{}',{status:401,headers:{'www-authenticate':'Basic realm="CRM"'}})}})
 try{
  const home=await invoke(handler,'/',{host:'imperiobarbaro.com.br'})
  assert.equal(home.code,200);assert.match(home.data,/Império Bárbaro/)
  assert.equal((await invoke(handler,'/home.css',{host:'www.imperiobarbaro.com.br'})).code,200)
  const head=await invoke(handler,'/',{host:'IMPERIOBARBARO.COM.BR:443'},'HEAD');assert.equal(head.code,200);assert.equal(head.data,'')
  for(const path of ['/api/contacts','/api/health','/index.html','/assets/private.js','/home.html','/missing']){
   assert.equal((await invoke(handler,path,{host:'imperiobarbaro.com.br',authorization:'Basic test'})).code,404)
  }
  assert.equal((await invoke(handler,'/',{host:'imperiobarbaro.com.br'},'POST')).code,405)
  assert.equal(calls,2)
  assert.equal((await invoke(handler,'/',{host:'dashboard.imperiobarbaro.com.br','x-forwarded-host':'imperiobarbaro.com.br'})).code,401)
  assert.equal((await invoke(handler,'/api/contacts',{host:'dashboard.imperiobarbaro.com.br'})).code,401)
  assert.equal(calls,4)
 }finally{await rm(dir,{recursive:true})}
})

test('Imagens públicas preservam bytes; PDFs removidos não são expostos',async()=>{
 const dir=await mkdtemp(join(tmpdir(),'home-assets-gateway-'))
 const bytes=Buffer.from([0x52,0x49,0x46,0x46,0x00,0xff,0x80,0x01])
 const files=new Map([['imperio-hero-768.webp','image/webp'],['imperio-hero-1536.webp','image/webp']])
 let calls=0
 const handler=createGateway({dist:dir,apiBase:'https://api.internal',fetcher:async()=>{calls++;return new Response('{}',{status:401})}})
 try{
  for(const name of [...files.keys(),'private.webp','private.pdf'])await writeFile(join(dir,name),bytes)
  for(const [name,type] of files){
   for(const method of ['GET','HEAD']){
    const result=await invoke(handler,'/'+name,{host:'imperiobarbaro.com.br'},method)
    assert.equal(result.code,200);assert.equal(result.headers['content-type'],type);assert.equal(result.headers['content-length'],bytes.length)
    assert.deepEqual(Buffer.concat(result.chunks),method==='GET'?bytes:Buffer.alloc(0))
   }
  }
  for(const path of ['/regras-principal.pdf','/regras-secundario.pdf','/private.webp','/private.pdf','/assets/imperio-hero-768.webp','/imperio-hero-3072.webp']){
   assert.equal((await invoke(handler,path,{host:'imperiobarbaro.com.br'})).code,404)
  }
  assert.equal(calls,0)
  for(const name of files.keys())assert.equal((await invoke(handler,'/'+name,{host:'dashboard.imperiobarbaro.com.br'})).code,401)
  assert.equal(calls,files.size)
 }finally{await rm(dir,{recursive:true})}
})

test('Leitura pública de liderança não encaminha credenciais e nunca permite gravação anônima',async()=>{
 const calls=[]
 const handler=createGateway({apiBase:'https://api.internal/base',fetcher:async(url,options)=>{calls.push({url:String(url),options});return new Response(JSON.stringify({revision:1,items:[]}))}})
 const headers={host:'imperiobarbaro.com.br',authorization:'Basic private','x-contacts-session':'secret'}
 const result=await invoke(handler,'/public/leadership?ignored=yes',headers)
 assert.equal(result.code,200);assert.equal(result.headers['cache-control'],'no-store')
 assert.equal(calls[0].url,'https://api.internal/base/public/leadership')
 assert.equal(calls[0].options.headers,undefined)
 assert.deepEqual(JSON.parse(result.data),{revision:1,items:[]})
 assert.equal((await invoke(handler,'/public/leadership',headers,'PUT','{}')).code,405)
 assert.equal((await invoke(handler,'/api/leadership',headers,'PUT','{}')).code,405)
 assert.equal((await invoke(handler,'/public/leadership/private',headers)).code,404)
 const head=await invoke(handler,'/public/leadership',headers,'HEAD');assert.equal(head.code,200);assert.equal(head.data,'')
 assert.equal(calls.length,2)
 const broken=createGateway({apiBase:'https://api.internal',fetcher:async()=>new Response('private error',{status:500})})
 const failed=await invoke(broken,'/public/leadership',headers);assert.equal(failed.code,502);assert.ok(!failed.data.includes('private error'))
})
