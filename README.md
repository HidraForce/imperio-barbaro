# Império Bárbaro CRM

Painel privado para dois clãs do Clash of Clans, com servidor Go, interface Vue em português, SQLite local e PostgreSQL na Vercel. O servidor entrega a API e a PWA no mesmo contêiner.

Para publicar usando a Vercel, siga o [guia de deploy na Vercel](README-VERCEL.md). Ele descreve a entrada web pública, a API interna e o banco PostgreSQL de produção.

## Iniciar com Docker

1. Configure `.env` com `CLASH_API_TOKEN`, `CLAN_TAG1` e `CLAN_TAG2`. Crie o token em [developer.clashofclans.com](https://developer.clashofclans.com/) para o IP público da máquina que acessará a API.
2. Defina `APP_USERNAME` e uma `APP_PASSWORD` privada com pelo menos 16 caracteres.
3. Execute `docker compose up --build -d`.
4. Abra [localhost:2025](http://localhost:2025) e entre com as credenciais configuradas.

Para acesso pela internet, use um proxy reverso com HTTPS: a autenticação HTTP Basic envia credenciais em todas as consultas. O token do Clash fica somente no servidor.

O banco está em um volume Docker e sobrevive às reconstruções. `docker compose down` encerra o aplicativo; `docker compose down -v` também exclui o banco.

## Configuração

O arquivo `.env` é privado e excluído do Git.

- `CLASH_API_TOKEN`: token da API do Clash.
- `CLAN_TAG1` / `CLAN_TAG2`: os dois clãs permitidos, com ou sem `#`. Um campo vazio desativa a opção correspondente.
- `APP_USERNAME` / `APP_PASSWORD`: credenciais compartilhadas dos líderes.
- `CONTACTS_PASSWORD`: senha exclusiva dos contatos, com pelo menos 16 caracteres e diferente de `APP_PASSWORD`. Sem ela, os contatos ficam bloqueados.
- `PORT`: porta do servidor; padrão `2025`.
- `DATABASE_PATH`: caminho do SQLite local; padrão Docker `/data/clan.db`.
- `DATABASE_URL`: conexão PostgreSQL. Quando definida, substitui o SQLite. Na Vercel, use a conexão Neon com `sslmode=verify-full`.
- `API_URL`: binding gerado pela Vercel para a entrada web chamar a API interna; não configure manualmente.

## Segurança

- Autenticação obrigatória para a página, os arquivos e todas as rotas da API, com comparação das credenciais em tempo constante.
- Senhas de exemplo e senhas com menos de 16 caracteres impedem a inicialização.
- Após dez tentativas inválidas em cinco minutos, novas tentativas daquele endereço são bloqueadas até o fim da janela. A resposta inclui `Retry-After`. O contador tem limite de memória.
- Gravações exigem `X-Clanboard-Request: 1`; cadastros aceitam somente JSON. Requisições de outros sites e subdomínios são rejeitadas. Não há permissões CORS.
- Política de conteúdo permite scripts apenas do próprio servidor, bloqueia enquadramento em outros sites e restringe conexões. Dados privados usam `Cache-Control: no-store`.
- O servidor limita cabeçalhos a 16 KiB e impõe tempos máximos de leitura e escrita.
- Contatos têm validação da tag, telefone e tamanho dos nomes e observações. JSON com campos desconhecidos ou múltiplos objetos é rejeitado. O corpo é limitado a 16 KiB; consultas SQL usam parâmetros.
- Exportações CSV neutralizam fórmulas em nomes e outros textos.
- O contêiner roda sem privilégios adicionais, com sistema de arquivos somente para leitura, todas as capacidades removidas e limite de processos. Apenas o volume de dados e o diretório temporário são graváveis.

O acesso continua compartilhado entre líderes. Não há contas individuais, MFA ou trilha de auditoria de alterações. O aplicativo continua dependendo de HTTPS no proxy para proteger as credenciais em trânsito. Se usar um proxy, preserve o cabeçalho `Host` original e configure HSTS nele; o servidor não confia em `X-Forwarded-For` nem em `X-Forwarded-Proto` enviados pelo cliente. O limite de tentativas usa o endereço da conexão direta, então usuários atrás do mesmo proxy compartilham esse limite.

Após atualizar, reconstrua o contêiner com `docker compose up --build -d` e recarregue o navegador para que ele envie o novo cabeçalho de proteção nas gravações. O volume existente é preservado.

## Proteção adicional dos contatos

A área **Contatos** exige uma segunda senha, além do acesso ao painel. Defina `CONTACTS_PASSWORD` no `.env` com pelo menos 16 caracteres e um valor diferente de `APP_PASSWORD`, depois reconstrua o contêiner com `docker compose up --build -d`. Senhas de exemplo são rejeitadas. Se a variável não estiver configurada, a área permanece bloqueada e o restante do painel funciona normalmente.

O desbloqueio usa um token aleatório de 256 bits, mantido apenas na memória da página. O servidor guarda somente o hash desse token. A sessão expira em dez minutos contados a partir do desbloqueio, sem renovação por consultas. No SQLite local, reiniciar o servidor invalida todas as sessões. No PostgreSQL, sessões e revogações são compartilhadas entre instâncias e permanecem até o prazo de expiração. Leitura, cadastro e exclusão exigem o token e a autenticação principal; a senha adicional não é enviada nas consultas seguintes.

A página bloqueia a área e limpa os dados carregados ao sair de **Contatos**, ocultar a aba, recarregar ou pressionar **Bloquear contatos**. A revogação da sessão é enviada ao servidor; se a rede impedir essa chamada, o token expira no prazo definido. Respostas que chegarem depois do bloqueio não repõem os dados na tela. O painel inicial não consulta a lista de contatos.

Após cinco senhas incorretas em cinco minutos, novas tentativas daquele endereço são bloqueadas até o fim da janela, com `Retry-After`. O limite usa o endereço da conexão direta, como a proteção do acesso principal. Tokens não são enviados em URLs, cookies, armazenamento local ou logs. Há no máximo 256 sessões simultâneas no servidor.

Essa camada controla o acesso pela aplicação. Os registros e backups do SQLite continuam precisando de proteção no servidor; a senha adicional não criptografa o banco e não constitui autenticação multifator. O acesso por HTTPS continua necessário.

Referência de implementação: [gerenciamento de sessões da OWASP](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html).

## Identidade visual

O painel usa uma interpretação própria da vila e dos recursos de Clash of Clans: pedra clara, madeira escura, ouro nos controles, verde e vermelho nos ataques, azul na liga e roxo como cor secundária. Superfícies planas, bordas quase retas, ícones vetoriais próprios e tabelas alinhadas substituem os gradientes, brilhos e cartões arredondados.

Referências: [apresentação oficial de Clash of Clans](https://supercell.com/en/games/clashofclans/), [história da Supercell — lançamento em agosto de 2012](https://supercell.com/en/about-us/) e [explicação da interface do jogo](https://supercell.com/en/games/clashofclans/blog/news/weve-revamped-the-clash-of-clans-user-interface/). As cores do painel são escolhas de design, não uma paleta oficial publicada.

## Cache da API

Todas as consultas externas passam por um cache compartilhado entre usuários: em memória no modo SQLite, ou no PostgreSQL entre instâncias na Vercel. A chave inclui o recurso completo, a tag e os parâmetros; clãs e guerras diferentes têm entradas independentes.

| Recurso | Validade |
| --- | --- |
| Perfil e membros do clã | 5 minutos |
| Guerra atual | 1 minuto |
| Grupo e guerras da Liga de Guerra de Clãs | 1 minuto |
| Fins de semana de incursões | 15 minutos |
| Erros da API | 30 segundos |

Consultas simultâneas ao mesmo recurso compartilham uma única chamada externa. **Sincronizar** e **Atualizar dados** reutilizam o cache enquanto estiver válido. Após a expiração, a próxima consulta busca dados novos. Não há atualização automática em segundo plano.

Uma resposta HTTP 429 pausa novas chamadas para toda a API, respeitando `Retry-After` em segundos ou como data HTTP; na ausência de um valor válido, a pausa dura um minuto. Durante a pausa, entradas válidas continuam disponíveis. Erros não substituem dados por um sucesso aparente, e dados expirados não são apresentados como atuais.

O cache mantém até 128 entradas e limita cada resposta externa a 8 MiB. No modo SQLite, reiniciar o servidor limpa o cache. No modo PostgreSQL, ele permanece disponível para as outras instâncias, respeitando a expiração. Os contatos e o histórico de incursões ficam persistidos no banco configurado. Respostas privadas não são armazenadas no cache HTTP do navegador.

## Página inicial

A **Visão geral** mostra uma tabela por jogador com as colunas **Incursões**, **Guerra** e **CWL**. Jogadores com pendências aparecem primeiro; **Só pendências** filtra a lista. Verde indica todos os ataques usados, vermelho indica ataques restantes ou não realizados após o encerramento, e cinza indica preparação, ausência de escalação ou dados indisponíveis. Ataques parciais continuam pendentes, incluindo ataques bônus nas incursões.

A tabela usa o fim de semana de incursões mais recente, a guerra atual e a rodada selecionada da liga. Apenas os jogadores escalados contam para guerra e CWL. Membros da lista histórica continuam visíveis mesmo após saírem do clã.

## Funcionalidades

- Perfil e lista de membros de cada clã configurado.
- Guerra atual: placar, ataques, estrelas e destruição.
- Liga de Guerra de Clãs: grupo, rodadas e escalações diárias.
- Incursões da capital: ataques, ouro e membros sem ataques registrados.
- Relatórios semanais em texto e CSV, em português.
- Contatos privados dos líderes e links para WhatsApp.
- Interface responsiva e PWA instalável.

Há um acesso compartilhado dos líderes; contas individuais e atribuições editáveis de ataques ainda não estão implementadas. Os contatos são compartilhados entre os dois clãs.

## Pessoas e contas nos contatos

Cada contato representa uma pessoa: nome ou apelido, WhatsApp e observações. Ela pode ter até 20 contas, em qualquer um dos dois clãs. Cada conta tem nome no jogo, tag e indicação de **Clã 1**, **Clã 2** ou **Clã não informado**. A lista mostra todas as contas vinculadas à pessoa; uma tag só pode pertencer a um contato.

Use **Editar pessoa e contas** para adicionar contas ou atualizar seu clã. **Reunir cadastros da mesma pessoa** une contatos antigos, mantendo todas as contas e preservando telefones diferentes e observações. A reunião é manual: nomes iguais não provocam união automática.

Na primeira inicialização após a atualização, o banco migra os contatos existentes, preservando nomes, tags, telefones e observações. Cada registro antigo começa como uma pessoa com uma conta; o clã fica como **não informado** até ser preenchido. A cópia e a remoção da tabela antiga acontecem na mesma transação. Tags antigas que se tornem duplicadas após a normalização interrompem a migração sem excluir os dados.

## Relatórios de incursões

Abra **Incursões**, selecione o fim de semana e baixe **Relatório semanal** ou **CSV**. Fins de semana ativos geram relatórios provisórios. Sincronize após o encerramento e a expiração do cache para obter os totais finais.

A consulta `/clans/{clanTag}/capitalraidseasons` retorna os dez fins de semana mais recentes. O aplicativo salva os registros por clã e exibe até 52 fins de semana. Abrir o painel ou sincronizar dispara a leitura, respeitando o cache; não há coleta agendada.

A lista de membros é registrada uma única vez por fim de semana, na primeira sincronização. Se um fim de semana antigo for consultado pela primeira vez após o término, a lista representa os membros disponíveis nessa consulta, e não uma composição histórica confirmada. Mudanças entre sincronizações, elegibilidade e ataques em outros clãs não são comprovados pelo relatório.

As datas da lista de membros e da última consulta de incursões refletem a obtenção dos dados da API; ler o cache não altera essas datas para aparentar uma nova consulta.

## Liga de Guerra de Clãs

O botão **Todos** consulta as rodadas já definidas e mostra a temporada completa do clã selecionado, com todos os inscritos e participantes históricos e uma coluna por rodada. Reservas aparecem como **Não escalado**; rodadas futuras ou com falha ficam sem obrigação confirmada. As consultas usam o cache existente. **Sincronizar** recarrega os dados.

**Exportar CSV** na CWL baixa a rodada selecionada; **Exportar CSV · Todos** inclui todas as rodadas, com reservas e indicação de dados indisponíveis. Na aba **Guerra**, **Exportar CSV** baixa a escalação da guerra atual. Os arquivos incluem clã, rodada, situação da batalha, jogador, tag, escalação, ataques usados e permitidos, pendências, estrelas, destruição total dos ataques e alvos. Nomes são protegidos contra interpretação como fórmulas em planilhas.

Abra **Liga** e escolha uma rodada para acompanhar o adversário, o placar e os jogadores que ainda não atacaram. Apenas a escalação daquela rodada é contabilizada; jogadores inscritos que estão no banco não contam como faltantes. Rodadas futuras mostram uma mensagem até que as guerras sejam definidas.

O servidor consulta `/clans/{clanTag}/currentwar/leaguegroup` e resolve as batalhas por `/clanwarleagues/wars/{warTag}`. Somente tags vindas do grupo atual do clã configurado são aceitas.

## Mensagens de inadimplência para WhatsApp

O gerador aparece em **Visão geral**, **Incursões**, **Guerra** e **Liga** e usa somente o clã selecionado. Na visão geral, combina as incursões mais recentes, a guerra atual e a rodada selecionada da CWL. Nas demais abas, usa o recorte exibido; **Todos** na liga considera a temporada inteira. Escolha todos com pendências ou uma conta específica e use **Copiar mensagem para WhatsApp** para colar o texto na conversa desejada.

Atividades encerradas com ataques restantes pedem justificativa à liderança e informam que, sem justificativa, o jogador ficará sob aviso. Atividades em andamento recebem um lembrete para atacar dentro do prazo; preparação, reservas e dados indisponíveis não geram cobrança. Nas incursões, o texto solicita conferência da elegibilidade e participação em outros clãs quando os registros não confirmam esses dados. Ataques parciais e bônus pendentes são incluídos quando o limite é conhecido.

As mensagens são geradas no navegador, sem envio automático, acesso aos contatos privados ou registro de punições. A seleção individual identifica a conta pela tag; contas da mesma pessoa não são reunidas por suposição.

## Desenvolvimento local

Compile a interface com `cd web`, `npm install` e `npm run build`. Na raiz, exporte as variáveis do `.env` e execute `go run .`. O Docker Compose é o caminho recomendado.

Para validar o projeto, execute `npm test` e `npm run build` em `web/` e `go test -race ./...` na raiz. Para verificar as proteções e a validação dos contatos sem as dependências externas do servidor, execute `go test -race database.go database_test.go shared_security.go security.go contact_validation.go contacts_security.go contacts_store.go security_test.go contacts_security_test.go contact_accounts_test.go`. Os testes de cache verificam consultas simultâneas, isolamento dos clãs, expiração, cancelamento, respostas inválidas e pausa após o limite da API.
