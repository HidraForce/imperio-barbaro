# Alternativa: Vercel encaminhando para Docker externo

Este guia mantém o aplicativo atual: Vue, servidor Go, contatos privados, histórico SQLite e cache da API. A Vercel será o endereço público e encaminhará as requisições ao aplicativo Docker em um servidor externo. **Você precisará desse servidor além da Vercel.**

A Vercel já oferece um [runtime Go, inclusive para servidores `net/http` e `chi`](https://vercel.com/docs/functions/runtimes/go). O bloqueio deste projeto é a persistência: as funções têm [sistema de arquivos somente para leitura e espaço temporário em `/tmp`](https://vercel.com/docs/functions/runtimes). Colocar o SQLite em `/tmp` não fornece armazenamento durável para contatos e histórico. Publicar somente `web/` também não publica a API nem a autenticação que hoje protege a página.

```text
Navegador → HTTPS / Vercel → HTTPS / Caddy no servidor → Docker / Go
                                                            ↓
                                                 volume persistente / SQLite
                                                            ↓
                                                 API do Clash of Clans
```

## 1. Prepare os endereços e o servidor

Tenha uma conta Vercel, um repositório Git privado com o projeto e um servidor Linux com Docker Compose, Caddy e IP de saída estável.

Neste exemplo:

| Endereço | Uso |
| --- | --- |
| `painel.seudominio.com` | Domínio público configurado na Vercel |
| `origem.seudominio.com` | Domínio apontado por DNS para o servidor Docker |

Substitua os dois endereços em todos os exemplos. Eles devem ser diferentes para evitar um ciclo de encaminhamento. Também é possível usar o domínio **de produção** `seu-projeto.vercel.app` no lugar de `painel.seudominio.com`.

No servidor, obtenha o projeto pelo seu repositório. Na raiz dele:

```sh
cp .env.example .env
chmod 600 .env
```

Edite `.env` no servidor:

```dotenv
CLASH_API_TOKEN=SEU_TOKEN_PRIVADO
CLAN_TAG1=TAG_DO_PRIMEIRO_CLA
CLAN_TAG2=TAG_DO_SEGUNDO_CLA
APP_USERNAME=SEU_USUARIO
APP_PASSWORD=SUA_SENHA_PRIVADA_COM_PELO_MENOS_16_CARACTERES
CONTACTS_PASSWORD=OUTRA_SENHA_PRIVADA_COM_PELO_MENOS_16_CARACTERES
PORT=2025
DATABASE_PATH=/data/clan.db
```

Os valores acima são marcadores: troque-os por valores reais. As duas senhas devem ser diferentes. Não envie `.env`, o banco ou backups para o Git; o projeto já ignora `.env` e `data/`. Mantenha backups fora do repositório.

No [portal da API do Clash](https://developer.clashofclans.com/), crie o token autorizando o **IP público de saída do servidor Docker**. É esse servidor que consulta o Clash, não o navegador nem a Vercel.

No `docker-compose.yml` da cópia no servidor, substitua o mapeamento de porta:

```yaml
ports:
  - "127.0.0.1:2025:2025"
```

Esse exemplo pressupõe Caddy instalado no próprio servidor, fora do Docker. A porta 2025 ficará acessível localmente; publique HTTPS pelo Caddy. Em seguida:

```sh
docker compose up --build -d
docker compose logs --tail=50 clanboard
```

O volume `clanboard-data` guarda `/data/clan.db`. Preserve esse volume nas atualizações. `docker compose down -v` exclui os dados.

## 2. Configure HTTPS e o domínio recebido pelo Go

Aponte `origem.seudominio.com` para o servidor e permita as portas 80 e 443 para o Caddy. Acrescente este bloco ao `/etc/caddy/Caddyfile`:

```caddyfile
origem.seudominio.com {
    header Strict-Transport-Security "max-age=31536000"
    reverse_proxy 127.0.0.1:2025 {
        header_up Host painel.seudominio.com
    }
}
```

Valide e recarregue a configuração:

```sh
sudo caddy validate --config /etc/caddy/Caddyfile
sudo systemctl reload caddy
```

O `Host` fixo deve ser **exatamente o domínio público de produção da Vercel**, sem `https://` ou caminho. O Go compara `Origin` com `Host` nas consultas da API. Esse ajuste evita erro 403 ao desbloquear, salvar ou reunir contatos por meio da Vercel. Ele usa o [controle de cabeçalhos do Caddy](https://caddyserver.com/docs/caddyfile/directives/reverse_proxy); não copie um `X-Forwarded-Host` arbitrário enviado pelo visitante e não remova a proteção de origem do aplicativo.

O proxy deve encaminhar `Authorization`, `Origin`, `Sec-Fetch-Site`, `X-Clanboard-Request` e `X-Contacts-Session`, além dos métodos, corpos e parâmetros das requisições. Preserve também as respostas de autenticação e os cabeçalhos de segurança do Go, inclusive `Cache-Control: no-store`.

## 3. Crie o projeto de encaminhamento na Vercel

Na sua cópia local do repositório, crie uma pasta separada para o projeto Vercel:

```sh
mkdir -p deploy/vercel/public
touch deploy/vercel/public/.gitkeep
```

Crie `deploy/vercel/vercel.json` com:

```json
{
  "$schema": "https://openapi.vercel.sh/vercel.json",
  "framework": null,
  "installCommand": "",
  "buildCommand": "",
  "outputDirectory": "public",
  "rewrites": [
    {
      "source": "/:path*",
      "destination": "https://origem.seudominio.com/:path*"
    }
  ]
}
```

Substitua o domínio de destino e envie essa pasta ao Git. A configuração usa [reescritas para uma origem externa](https://vercel.com/docs/routing/rewrites). Ela encaminha página, arquivos e `/api/` ao Go, mantendo o endereço da Vercel no navegador e o login principal antes da página.

Na Vercel:

1. Abra **Add New → Project** e importe o repositório.
2. Defina **Root Directory** como `deploy/vercel`.
3. Escolha **Framework Preset: Other**. Os comandos e a saída são definidos pelo JSON acima; não configure o build do Vue neste projeto de encaminhamento.
4. Publique o projeto e configure `painel.seudominio.com` em **Settings → Domains**. Siga os registros DNS indicados pela Vercel.
5. Se escolher `seu-projeto.vercel.app` como endereço público, ajuste o `Host` no Caddy para esse nome e recarregue sua configuração.

Os segredos da aplicação ficam no servidor Docker. **Não crie variáveis `VITE_` com tokens ou senhas.** Não há variáveis da aplicação a configurar na Vercel nesta arquitetura.

A configuração de `Host` atende a um domínio público por vez. Use o endereço canônico de produção; URLs diferentes de preview podem abrir a página, mas as gravações com `Origin` diferente serão recusadas. Para testar alterações em preview com gravações, configure um ambiente de teste separado, com outro backend, banco e domínio canônico.

## 4. Confira o funcionamento após publicar

Abra o domínio público em uma janela privada. O navegador deve pedir `APP_USERNAME` e `APP_PASSWORD` antes de mostrar o painel. Confira os dois clãs, guerra, CWL, incursões e os downloads CSV.

Abra **Contatos**: a segunda senha deve ser necessária. Teste cadastrar e editar uma pessoa de teste, vincular contas e bloquear a área. Recrie o contêiner e confira que o cadastro continua no banco. Isso valida tanto as gravações pelo proxy quanto a persistência.

Para verificar a API sem colocar a senha no comando ou na URL, substitua o usuário abaixo; o `curl` pedirá a senha:

```sh
curl --user SEU_USUARIO https://painel.seudominio.com/api/health
```

A resposta autenticada deve ser `{"status":"ok"}`. Confira também os cabeçalhos:

```sh
curl --head --user SEU_USUARIO https://painel.seudominio.com/
```

Verifique `Cache-Control: no-store` e os cabeçalhos de segurança. Não habilite cache de CDN para as respostas privadas. O cache que reduz consultas ao Clash continua no processo Go.

## 5. Atualizações e erros comuns

Atualizações da aplicação acontecem no servidor Docker: obtenha o código atualizado e execute `docker compose up --build -d`. Mudanças no encaminhamento ou domínio também exigem atualizar a configuração correspondente na Vercel/Caddy.

| Problema | O que verificar |
| --- | --- |
| 403 ao desbloquear ou salvar contatos | `Host` fixo no Caddy igual ao domínio aberto no navegador; cabeçalhos preservados |
| 502 ou destino indisponível | DNS e certificado da origem, Caddy ativo e contêiner escutando na porta 2025 |
| Login reaparece ou API responde 401 | Encaminhamento do cabeçalho `Authorization` e credenciais principais |
| Contatos bloqueados | `CONTACTS_PASSWORD` válida, diferente da senha principal; recrie o contêiner após editar `.env` |
| API do Clash recusa o token | IP de saída autorizado no portal do Clash e token correto no servidor |
| Dados desaparecem | Volume `/data` preservado e `DATABASE_PATH=/data/clan.db` |
| Muitas tentativas bloqueiam todos | O Go limita pelo IP da conexão direta; atrás do Caddy, os usuários compartilham esse limite |
| Página abre, mas não tem dados | A página e `/api/` devem apontar para a mesma origem Docker; não publique só `web/dist` |

Faça backup consistente do diretório `/data`, incluindo eventuais arquivos WAL do SQLite. Para uma cópia simples, interrompa o contêiner durante o backup e preserve o diretório inteiro; depois inicie-o novamente. Uma reversão de código não desfaz migrações do banco: guarde um backup antes de atualizar.

## Se quiser executar a aplicação diretamente na Vercel

Esse caminho exige alterações de código e **não está implementado neste projeto**:

- Substituir o SQLite local por um banco persistente remoto, adaptando consultas, transações e migrações.
- Compartilhar sessões dos contatos, limites de tentativas e cache/cooldown entre instâncias. Hoje ficam na memória de um processo; a distribuição das requisições pode invalidar sessões e multiplicar consultas ao Clash.
- Definir uma saída de rede compatível com os IPs autorizados do token do Clash.
- Configurar o build para gerar `web/dist` antes de compilar o Go, que incorpora esses arquivos com `go:embed`.
- Preparar e versionar as dependências Go, incluindo `go.sum`, e configurar o runtime Go e os segredos no ambiente de produção.

A simples troca de `DATABASE_PATH` por `/tmp/clan.db` ou a inclusão de um `vercel.json` na raiz não resolve esses pontos. O procedimento acima mantém o banco e o servidor atuais e usa a Vercel como entrada pública.

Documentação conferida em 8 de outubro de 2026. Este guia não representa um deploy já realizado: domínios, credenciais e infraestrutura precisam ser configurados e verificados no seu ambiente.
