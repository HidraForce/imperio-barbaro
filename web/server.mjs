import { createServer } from 'node:http'
import { fileURLToPath } from 'node:url'
import { createGateway } from './gateway.mjs'
if(!process.env.API_URL)throw Error('API_URL ausente. Execute vercel dev para receber o binding interno.')
const server=createServer(createGateway({apiBase:process.env.API_URL,dist:fileURLToPath(new URL('./dist',import.meta.url))}))
server.headersTimeout=10000;server.requestTimeout=90000;server.keepAliveTimeout=5000
server.listen(Number(process.env.PORT||3000),'0.0.0.0')
