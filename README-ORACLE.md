# Deploy na Oracle

Produção: Ubuntu ARM64, Docker Compose, Caddy com HTTPS e SQLite no volume `imperio-barbaro-oracle_clanboard-data`. O checkout é `~/imperio-barbaro-git`; a configuração privada fica em `.env.oracle`.

## Ativar atualizações automáticas

Depois de mesclar o PR em `main`, execute uma vez na VM como o usuário `ubuntu`:

```bash
cd ~/imperio-barbaro-git
git pull --ff-only origin main
bash scripts/oracle-install-deploy.sh
```

O instalador precisa de Python 3.12+, Git, Docker Compose e `sudo` para registrar o serviço e o timer. O deploy roda como o dono do checkout, com acesso ao Docker.

Daí em diante: **branch → PR → testes aprovados → merge em `main`**. A VM procura novas versões a cada cinco minutos, com até trinta segundos extras de intervalo. Ela espera o workflow `tests.yml` aprovar o commit exato de `main`; falhas de testes, rede ou API impedem a atualização.

Cada release compila uma imagem identificada pelo commit, confere o volume existente, cria um backup e recria os serviços. A validação exige site público com HTTP 200, painel sem login com 401 e `/api/health` autenticado com 200. Backups e recriação interrompem o serviço brevemente. A implantação não precisa de chave SSH ou segredo de produção no GitHub.

Se a ativação falhar, o serviço restaura e verifica o código anterior. O banco é preservado: alterações incompatíveis no esquema exigem recuperação própria. A mesma versão com falha fica bloqueada até investigação e nova tentativa manual.

```bash
python3 scripts/oracle-deploy.py --check
sudo systemctl start imperio-deploy.service
journalctl -u imperio-deploy.service -n 100 --no-pager
systemctl list-timers imperio-deploy.timer --no-pager
```

Para repetir uma versão que falhou: `python3 scripts/oracle-deploy.py --retry`.
Para pausar futuras atualizações: `sudo systemctl disable --now imperio-deploy.timer`.
Um deploy em andamento continua até terminar. Não execute outros comandos de atualização durante o deploy.

O estado e as cópias do código ficam em `.deploy/`; imagens e backups são mantidos para recuperação. Acompanhe o espaço em disco, copie backups para fora da VM e remova versões antigas somente após conferir quais diretórios estão montados pelo Caddy. Mantenha a versão atual, a anterior e os backups necessários. O serviço registra falhas no journal; não envia notificações.

## Configuração de produção

Para uma VM nova, clone o repositório e crie `.env.oracle` a partir de `deploy/oracle/.env.example`, com permissão 600. Configure:

| Variável | Valor |
| --- | --- |
| `DOMAIN` | `imperiobarbaro.com.br` |
| `DASHBOARD_DOMAIN` | `dashboard.imperiobarbaro.com.br` |
| `ACME_EMAIL` | E-mail para o certificado HTTPS |
| `CLASH_API_TOKEN` | Token com o IP público da VM autorizado |
| `CLAN_TAG1`, `CLAN_TAG2` | Tags dos clãs |
| `APP_USERNAME`, `APP_PASSWORD` | Login do painel |
| `CONTACTS_PASSWORD` | Senha separada dos contatos |

Os registros DNS A dos dois domínios devem apontar para o IP público reservado da VM. Libere TCP 80 e 443 para a internet e restrinja SSH ao seu IP. Não exponha a porta 2025. O Compose de produção configura o SQLite e seu volume; não combine esse arquivo com o Compose de desenvolvimento.

```bash
bash scripts/oracle-compose.sh config -q
bash scripts/oracle-compose.sh up -d --build
bash scripts/oracle-compose.sh ps
```

O site deve abrir sem login; o painel deve pedir as credenciais existentes. Confira os clãs, o desbloqueio dos contatos e os dados após reiniciar o serviço.

## Backup e recuperação

```bash
bash scripts/oracle-backup.sh
```

O backup para brevemente o aplicativo, copia todo o volume SQLite e reinicia o serviço. O script compartilha um bloqueio com o deploy automático. Os arquivos privados ficam em `backups/`; copie-os para fora da VM e guarde `.env.oracle` separadamente. Backups contêm dados dos contatos.

Para recuperação, prepare outra VM ou um volume vazio e restaure somente um arquivo confiável:

```bash
bash scripts/oracle-compose.sh build clanboard
bash scripts/oracle-compose.sh create clanboard
bash scripts/oracle-compose.sh run --rm --no-deps -T --entrypoint sh clanboard -c \
  'test -z "$(ls -A /data)" || exit 1; tar -xzf - -C /data' \
  < backups/SEU_BACKUP.tar.gz
bash scripts/oracle-compose.sh up -d
```

Confira contatos, histórico de raids e saúde autenticada antes de substituir a instalação anterior. Não restaure sobre o banco ativo nem use `down -v` ou `docker volume prune` em produção.

## Atualização manual

Pause o timer antes de fazer uma atualização manual. Se o deploy automático já foi usado, consulte `image_tag` em `.deploy/state.json`; `.env.oracle` não é reescrito pelo pipeline. Guarde essa imagem para rollback.

```bash
bash scripts/oracle-backup.sh
git pull --ff-only origin main
```

Defina um novo `APP_IMAGE_TAG` em `.env.oracle`, compile com `bash scripts/oracle-compose.sh build clanboard` e aplique com `bash scripts/oracle-compose.sh up -d --no-build`. Para voltar ao código anterior, use a tag retida e repita `up -d --no-build`. Isso não desfaz migrações do banco. Não remova o volume existente ao trocar o checkout.
