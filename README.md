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
| `NOTIFICATIONS_PASSWORD` | Senha exclusiva dos avisos personalizados, mínimo 16 caracteres e diferente de APP_PASSWORD. Sem ela, os avisos personalizados ficam bloqueados |
| `SUPER_NOTIFICATIONS_PASSWORD` | Senha para liberar um intervalo antes de uma hora, mínimo 16 caracteres e diferente de APP_PASSWORD. Sem ela, a liberação fica bloqueada |
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
- Regras públicas editáveis, com links externos opcionais para regulamentos e links por tópico, como `https://imperiobarbaro.com.br/#regras-principal-guerras`.
- Interface em português, responsiva e instalável como PWA.

O site público não contém links para o painel. O painel continua com a autenticação existente. O acesso dos líderes é compartilhado; não há contas individuais ou autenticação multifator.

## App dos membros e comunicados

Compartilhe `https://imperiobarbaro.com.br/instalar` com novos membros. O app abre em `/app`, recebe notificações Web Push e mostra os últimos 10 comunicados do clã escolhido. A página pública também oferece o link de instalação. O painel mantém sua autenticação e tem a seção **Comunicados**.

- Cinco mensagens prontas: guerra com zero ataques, segundo ataque da guerra, raids, jogos do clã e CWL (um ataque por rodada). As notificações são coletivas para o clã escolhido; não há vínculo individual entre celular e conta do jogo. O texto de guerra indica a quem se aplica.
- Os dois botões de guerra exigem uma guerra normal em andamento com dois ataques por membro. O segundo exige 12 horas completas desde o início, conferidas no servidor. Os outros lembretes são enviados por decisão da liderança.
- Cada botão, incluindo o personalizado, tem intervalo de uma hora por clã, aplicado no banco e compartilhado entre líderes, abas e reinícios. Sem inscritos, o envio não consome o intervalo.
- O histórico mantém os 10 comunicados mais recentes por clã. Ao publicar, ao iniciar o servidor e a cada rodada da fila, avisos excedentes e seus registros de entrega são apagados do banco. Envios pendentes ou em processamento são preservados até terminar ou expirar. As inscrições, chaves VAPID e intervalos de envio continuam preservados; apagar o histórico não libera um novo envio antes de uma hora. Essa limpeza não remove notificações já exibidas nos celulares nem apaga cópias dos backups.
- O aviso personalizado exige NOTIFICATIONS_PASSWORD; a liberação antecipada do intervalo exige SUPER_NOTIFICATIONS_PASSWORD. As três sessões (contatos, personalizado e intervalo) são independentes, expiram em dez minutos e bloqueiam ao ocultar o painel. Cada senha tem limite de tentativas e é verificada com PBKDF2-HMAC-SHA256 (600 mil iterações) e salt aleatório por acesso. Em PostgreSQL, a tabela `vault_salts` mantém esses salts entre instâncias e reinícios; nenhuma senha é armazenada nela. Liberar um intervalo não envia mensagem: apenas libera o botão escolhido naquele clã, e o próximo envio reinicia a hora. A regra das 12 horas de guerra permanece obrigatória.
- A ativação inclui todas as categorias. No Android compatível, o botão de instalação só é oferecido após a permissão e a inscrição no servidor. No iPhone/iPad é obrigatório adicionar à tela inicial primeiro e ativar os avisos ao abrir pelo ícone. O navegador pode oferecer sua própria instalação fora desse fluxo.
- Web Push usa `Urgency: high`, pede som e destaque quando suportados e respeita as configurações do aparelho. Não é possível forçar prioridade, som, permissão nem ignorar modos de foco. A inscrição confirma a configuração, não garante entrega ou leitura. O teste da página exibe uma notificação **local**; valide também um envio real do painel num Android e num iPhone em HTTPS.

O botão “Testar aviso neste celular” envia um push real pelo servidor, com o mesmo formato e prioridade dos comunicados. Ele exige a inscrição ativa e seu segredo, atinge somente aquele aparelho, permite um teste por minuto e não altera o histórico nem o intervalo dos comunicados. A tabela temporária `push_test_limits` é limpa automaticamente. O teste confirma aceitação pelo serviço, não exibição nem leitura. Abrir o painel de liderança não inscreve o celular: o líder também deve preparar o app de membros no mesmo clã. O painel e o app usam o mesmo service worker por origem, para uma visita ao painel não substituir o receptor de push.

As chaves VAPID são criadas automaticamente e persistidas na tabela `push_keys`; preserve o banco e os backups para manter as inscrições. Não é necessária conta Firebase ou loja de aplicativos. As tabelas `push_subscriptions`, `announcements`, `announcement_cooldowns` e `push_jobs` armazenam inscrições, histórico, intervalos e fila. O servidor tenta a entrega a cada dez segundos, com até seis tentativas e validade de uma hora; remove inscrições expiradas (404/410). Os contadores do painel distinguem fila, aceitação pelo serviço e falhas. A fila requer o processo Go permanente da implantação Oracle; um ambiente que suspende o processo precisa de um worker equivalente.

As únicas rotas públicas de notificações são `GET /api/push/config`, `GET /api/push/announcements?clan=1|2`, `POST /api/push/subscription`, `DELETE /api/push/subscription` e `POST /api/push/test`. Gravações exigem origem correspondente, JSON e o cabeçalho de proteção. Endpoints de inscrição são restritos aos serviços de push conhecidos para impedir requisições a hosts arbitrários. Os comunicados publicados são públicos por clã, sem telefones ou dados disciplinares. O exportador legado SQLite→PostgreSQL cobre contatos e raids; para migrar o app com inscrições, preserve também as tabelas de push (especialmente as chaves VAPID).

Os ícones PNG são derivados do escudo existente. Para regenerar: `python3 scripts/generate-app-icons.py`.

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

## Regras, avisos e punições

Em **Regras**, edite textos, tópicos, resumos dos clãs e links HTTPS dos regulamentos. Salvar publica diretamente no site; os PDFs antigos foram removidos. Links de tópicos existentes continuam funcionando, exceto se o tópico for removido.

Em **Avisos e punições**, registre ocorrências manualmente, informe justificativas e remova entradas das listas. Os avisos têm quatro setores: Jogos, Capital, Guerras e Replays. O grau é calculado pelos avisos ativos: grau 1 = advertência; grau 2 = fora de duas guerras ou perda de cargo; grau 3 = reincidência; grau 4 = expulsão, somente ao acumular quatro avisos no mesmo setor ou ao menos um em cada um dos quatro setores. Avisos de setores diferentes se somam, mas quatro avisos distribuídos em apenas dois ou três setores não autorizam expulsão. A API valida os requisitos ao registrar uma punição ativa; a liderança escolhe e executa a medida no jogo. O resumo também aparece em **Visão geral**.

Os avisos expiram após 30 dias sem reincidência, contados a partir da confirmação do aviso como ativo. Um novo aviso confirmado, em qualquer setor, reinicia o prazo de todos os avisos ainda ativos do membro, somente naquele clã. Avisos já expirados não são reativados. Registros a conferir, justificados, encerrados e removidos não contam e não prolongam o prazo. A expiração é calculada na consulta e mantém o histórico. Avisos automáticos de guerra/CWL pertencem a Guerras e raids pertencem a Capital. Avisos manuais antigos sem setor ficam a conferir até sua classificação.

Em **Membros**, ordene por troféus ou cargo, nos dois sentidos. A hierarquia é Líder, Colíder, Ancião e Membro; empates de cargo usam troféus decrescentes. A exportação CSV acompanha a ordem exibida.

A consulta automática é opcional por clã e roda no servidor a cada quinze minutos, mesmo com o painel fechado. Ela considera somente zero ataques de jogadores escalados em guerras/CWL encerradas. Nas raids, qualquer ataque disponível não usado (inclusive bônus explicitamente liberado) gera ocorrência a conferir, sem virar aviso ativo antes da revisão de elegibilidade; membros ausentes dos ataques só são identificados quando há uma lista capturada durante o evento. A primeira consulta cobre os últimos sete dias ainda disponíveis na API. A API precisa confirmar o estado encerrado e um horário de término já passado, numa resposta obtida após o evento. Escalações e totais incompletos ou divergentes não são interpretados como zero. Reservas e ataques parciais em guerra/CWL não geram avisos nem cobranças de justificativa. A conferência atualiza contagens de ocorrências automáticas ainda não revisadas e encerra registros quando os ataques finais comprovam que a cobrança não se aplica. Decisões já alteradas pela liderança ficam preservadas; divergências são sinalizadas para conferência. Remover uma ocorrência impede sua recriação na sincronização. Mensagens podem ser copiadas; não há envio automático ou expulsão pelo aplicativo.
