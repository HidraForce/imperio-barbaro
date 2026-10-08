import test from 'node:test'
import assert from 'node:assert/strict'
import { Readable, Writable } from 'node:stream'
import { mkdtemp, writeFile, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { createGateway, bindingURL } from './gateway.mjs'
class Reply extends Writable {
 constructor(){super();this.headersSent=false;this.data='';this.code=0;this.headers={}}
 writeHead(code,headers){this.code=code;this.headers=headers;this.headersSent=true}
 _write(chunk,encoding,done){this.data+=chunk.toString();done()}
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
