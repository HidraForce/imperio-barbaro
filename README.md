# Império Bárbaro CRM

Site da comunidade e painel privado para os líderes dos dois clãs. Go serve a API e a interface Vue no mesmo contêiner; a produção usa Oracle Cloud, Caddy com HTTPS e SQLite em volume Docker.

- Site público: https://imperiobarbaro.com.br
- Painel: https://dashboard.imperiobarbaro.com.br
- [Deploy e atualizações automáticas](README-ORACLE.md)

## Rodar localmente

```bash
cp .env.example .env
docker compose up --build -d
```

Preencha `.env` antes de iniciar e abra http://localhost:2025.

| Variável | Uso |
| --- | --- |
| `CLASH_API_TOKEN` | Token criado em https://developer.clashofclans.com com o IP público do servidor autorizado |
| `CLAN_TAG1`, `CLAN_TAG2` | Tags dos dois clãs; um campo vazio desativa aquele clã |
| `APP_USERNAME`, `APP_PASSWORD` | Login compartilhado dos líderes; senha privada com pelo menos 16 caracteres |
| `CONTACTS_PASSWORD` | Senha dos contatos, diferente da principal e com pelo menos 16 caracteres |
| `PORT` | Porta do servidor; padrão `2025` |
| `DATABASE_PATH` | Caminho do SQLite; no Docker, `/data/clan.db` |
| `DATABASE_URL` | PostgreSQL opcional; quando definido, substitui o SQLite |

O token e as senhas ficam no servidor. Não comite arquivos de ambiente, bancos ou backups. `docker compose down -v` apaga o volume do banco.

## Funcionalidades

- Visão geral com pendências de raids, guerra e CWL: verde para ataques concluídos, vermelho para pendentes e cinza para ausência de escalação ou dados.
- Guerra atual, rodadas da CWL e visão de toda a temporada, com exportação CSV.
- Histórico de até 52 fins de semana de raids e relatórios em texto e CSV. A lista de membros é capturada na primeira sincronização; fins de semana antigos podem não refletir a composição histórica do clã.
- Mensagens coletivas ou individuais para copiar no WhatsApp, incluindo filtros de zero e um ataque na guerra. O aplicativo não envia mensagens automaticamente.
- Contatos por pessoa, com até 20 contas nos dois clãs e links para WhatsApp. A área exige a segunda senha, bloqueia ao sair ou ocultar a aba e expira em dez minutos.
- Liderança editável no painel, publicada no site com cargos, responsáveis e vagas.
- Regras públicas com PDFs e links por tópico, como `https://imperiobarbaro.com.br/#regras-principal-guerras`.
- Interface em português, responsiva e instalável como PWA.

O site público não contém links para o painel. O painel continua com a autenticação existente. O acesso dos líderes é compartilhado; não há contas individuais ou autenticação multifator.

## Cache e segurança

O cache da API dura cinco minutos para membros, um minuto para guerra/CWL e quinze minutos para raids. Consultas simultâneas compartilham a mesma chamada, e respostas 429 respeitam `Retry-After`. A sincronização acontece ao consultar o painel, sem coleta agendada.

O painel exige HTTPS em produção, limita tentativas de login e rejeita gravações de outras origens. Contatos usam uma sessão separada; essa proteção não criptografa o banco nem seus backups. O contêiner roda sem privilégios adicionais e persiste dados somente no volume.

## Desenvolvimento e testes

```bash
cd web
npm ci
npm test
npm run build
cd ..
python3 -m unittest discover -s scripts -p '*_test.py' -v
go mod tidy
go test -race ./...
go build ./...
```

Os testes de PostgreSQL usam `TEST_DATABASE_URL`; o workflow de CI já fornece um banco descartável. Para trabalhar na interface, execute `npm run dev` em `web/`.
