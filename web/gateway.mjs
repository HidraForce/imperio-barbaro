import { createReadStream } from 'node:fs'
import { stat } from 'node:fs/promises'
import { resolve, extname, sep } from 'node:path'
import { Readable } from 'node:stream'
import { pipeline } from 'node:stream/promises'

const types={'.html':'text/html; charset=utf-8','.js':'application/javascript; charset=utf-8','.css':'text/css; charset=utf-8','.json':'application/json; charset=utf-8','.webmanifest':'application/manifest+json; charset=utf-8','.svg':'image/svg+xml','.png':'image/png','.webp':'image/webp','.pdf':'application/pdf','.ico':'image/x-icon'}
export function bindingURL(value){
 const url=new URL(value)
 if(!['http:','https:'].includes(url.protocol)||url.username||url.password||url.hash||url.search)throw Error('API_URL deve ser fornecida pelo binding interno da Vercel.')
 return url
}
function apiURL(base,path,query=''){const url=new URL(base);url.pathname=base.pathname.replace(/\/$/,'')+path;url.search=query;return url}
function fail(res,status,message){res.writeHead(status,{'content-type':'application/json; charset=utf-8','cache-control':'no-store'});res.end(JSON.stringify({error:message}))}
function allowedOrigin(req){
 if(['cross-site','same-site'].includes(req.headers['sec-fetch-site']))return false
 if(!req.headers.origin)return true
 try{const origin=new URL(req.headers.origin);return ['http:','https:'].includes(origin.protocol)&&origin.host.toLowerCase()===String(req.headers.host).toLowerCase()&&origin.pathname==='/'&&!origin.username&&!origin.password&&!origin.search&&!origin.hash}catch{return false}
}
function responseHeaders(upstream){
 const headers={};for(const [key,value] of upstream.headers){if(!['content-length','transfer-encoding','connection','set-cookie','location','content-encoding'].includes(key))headers[key]=value}
 headers['cache-control']='no-store';return headers
}
export function createGateway({apiBase,dist=resolve('dist'),fetcher=fetch}){
 const base=bindingURL(apiBase),root=resolve(dist)
 return async function gateway(req,res){
  let path,query
  try{const parsed=new URL(req.url,'http://crm.local');path=decodeURIComponent(parsed.pathname);query=parsed.search}catch{fail(res,400,'Endereço inválido.');return}
  // This is the only anonymous dynamic resource: public website leadership.
  if(path==='/public/leadership'){
   if(!['GET','HEAD'].includes(req.method)){fail(res,405,'Método não permitido.');return}
   try{
    const upstream=await fetcher(apiURL(base,'/public/leadership'),{method:'GET',redirect:'manual',signal:AbortSignal.timeout(10000)})
    if(!upstream.ok){fail(res,502,'Não foi possível carregar a liderança pública.');return}
    const data=await upstream.json()
    res.writeHead(200,{'content-type':'application/json; charset=utf-8','cache-control':'no-store','x-content-type-options':'nosniff'})
    res.end(req.method==='HEAD'?undefined:JSON.stringify(data))
   }catch{fail(res,502,'Não foi possível carregar a liderança pública.')}
   return
  }
  const host=String(req.headers.host||'').toLowerCase().replace(/:\d+$/,'')
  if(['imperiobarbaro.com.br','www.imperiobarbaro.com.br'].includes(host)){
   if(!['GET','HEAD'].includes(req.method)){fail(res,405,'Método não permitido.');return}
   const name=new Map([['/','home.html'],['/home.css','home.css'],['/home.js','home.js'],['/icon.svg','icon.svg'],['/sw.js','sw.js'],['/imperio-hero-768.webp','imperio-hero-768.webp'],['/imperio-hero-1536.webp','imperio-hero-1536.webp'],['/regras-principal.pdf','regras-principal.pdf'],['/regras-secundario.pdf','regras-secundario.pdf']]).get(path)
   if(!name){fail(res,404,'Página não encontrada.');return}
   const filename=resolve(root,name),info=await stat(filename).catch(()=>null)
   if(!info?.isFile()){fail(res,404,'Arquivo não encontrado.');return}
   res.writeHead(200,{'content-type':types[extname(filename)],'content-length':info.size,'cache-control':'no-store','x-content-type-options':'nosniff','x-frame-options':'DENY','referrer-policy':'no-referrer','content-security-policy':"default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'"})
   if(req.method==='HEAD')res.end();else await pipeline(createReadStream(filename),res)
   return
  }
  const isAPI=path==='/api'||path.startsWith('/api/')
  if(isAPI&&!allowedOrigin(req)){fail(res,403,'Esta consulta deve ser feita pelo próprio painel.');return}
  if(isAPI&&!['GET','HEAD'].includes(req.method)&&req.headers['x-clanboard-request']!=='1'){fail(res,403,'Proteção de gravação ausente. Atualize o painel.');return}
  if(!isAPI&&!['GET','HEAD'].includes(req.method)){fail(res,405,'Método não permitido.');return}
  const controller=new AbortController(),timeout=setTimeout(()=>controller.abort(),90000)
  req.once('aborted',()=>controller.abort());res.once('close',()=>{if(!res.writableEnded)controller.abort()})
  const headers=new Headers()
  for(const name of ['authorization','content-type','x-clanboard-request','x-contacts-session']){if(req.headers[name])headers.set(name,req.headers[name])}
  try{
   // Origin é conferida no serviço público. A chamada seguinte é interna e
   // usa o Host do binding; cabeçalhos encaminhados do visitante não são confiados.
   const upstream=await fetcher(apiURL(base,isAPI?path:'/api/health',isAPI?query:''),{method:isAPI?req.method:'GET',headers,body:isAPI&&!['GET','HEAD'].includes(req.method)?req:undefined,duplex:'half',redirect:'manual',signal:controller.signal})
   if(isAPI||!upstream.ok){
    res.writeHead(upstream.status,responseHeaders(upstream))
    if(req.method==='HEAD'||!upstream.body)res.end();else await pipeline(Readable.fromWeb(upstream.body),res)
    return
   }
   await upstream.body?.cancel()
   let filename=resolve(root,'.'+path)
   if(filename!==root&&!filename.startsWith(root+sep)){fail(res,404,'Arquivo não encontrado.');return}
   let info=await stat(filename).catch(()=>null)
   if(info?.isDirectory()){filename=resolve(filename,'index.html');info=await stat(filename).catch(()=>null)}
   if(!info?.isFile()&&!extname(path)){filename=resolve(root,'index.html');info=await stat(filename).catch(()=>null)}
   if(!info?.isFile()||path.split('/').some(part=>part.startsWith('.'))){fail(res,404,'Arquivo não encontrado.');return}
   const outgoing=responseHeaders(upstream);outgoing['content-type']=types[extname(filename)]||'application/octet-stream';outgoing['content-length']=info.size
   res.writeHead(200,outgoing)
   if(req.method==='HEAD')res.end();else await pipeline(createReadStream(filename),res)
  }catch{if(!res.headersSent)fail(res,502,'Não foi possível acessar a API interna. Tente novamente.');else res.destroy()}
  finally{clearTimeout(timeout)}
 }
}
