# Oracle Cloud Always Free deployment

Run the existing Go/Vue image and Caddy on one VM. Go serves both the UI and API; SQLite stores contacts and raid history in a Docker volume. Caddy issues and renews HTTPS certificates. No separate frontend service, managed database, Kubernetes cluster, or load balancer is required.

## 1. Resources to create

In your OCI **home region**, create these resources in a dedicated compartment:

| Resource | Configuration |
| --- | --- |
| Compute | `VM.Standard.A1.Flex`, **1 OCPU / 6 GB RAM**, Always Free eligible Ubuntu 24.04 ARM64 image |
| Boot volume | **50 GB**, Always Free eligible settings; stores Docker and the database |
| Network | VCN, public subnet, internet gateway, route `0.0.0.0/0` to that gateway |
| Public IP | Prefer a reserved IPv4 attached to the VM, to keep DNS and the Clash API allowlist stable |
| DNS | A records for `imperiobarbaro.com.br` and `dashboard.imperiobarbaro.com.br` pointing directly to that IPv4 |

As checked on **2026-10-13**, [Oracle's Always Free documentation](https://docs.oracle.com/en-us/iaas/Content/FreeTier/freetier_topic-Always_Free_Resources.htm) lists 1,500 A1 OCPU-hours and 9,000 GB-hours monthly (2 OCPUs / 12 GB total), 200 GB combined boot/block storage, and five volume backups. Other instances consume the same allowances. Check the console's eligibility and cost estimate before creating resources; older guides advertise larger A1 allowances. The trial credit is not the ongoing free allowance.

Oracle may reclaim idle free instances, and A1 capacity can be unavailable. Keep an off-VM backup and retain the boot volume if replacing the VM. If capacity is unavailable, retry another availability domain in the home region or later; do not substitute a paid shape. A domain may cost money separately; an existing domain or a DNS provider's free hostname can be used.

Create **stateful ingress rules** in the attached network security group or subnet security list:

| Source | Protocol / destination port | Purpose |
| --- | --- | --- |
| Your public IP `/32` | TCP 22 | SSH administration |
| `0.0.0.0/0` | TCP 80 | Certificate validation and HTTPS redirect |
| `0.0.0.0/0` | TCP 443 | Application HTTPS |

Allow outbound traffic for DNS, HTTPS APIs, package installation, and certificate renewal (the default outbound rule is sufficient). Do not expose port 2025 or the Docker daemon. Check host firewall rules too; Docker-published ports use forwarding rules, so UFW alone is not an access boundary. Preserve OCI image firewall rules and use the OCI ingress rules to restrict access. Do not flush iptables. Publish no AAAA record unless IPv6 routing and firewall rules are also configured.

## 2. Prepare the VM

Connect using the SSH key selected when creating the instance:

```bash
ssh -i /path/to/private-key ubuntu@YOUR_VM_IP
sudo apt-get update
sudo apt-get install -y ca-certificates curl git openssl
```

Install Docker Engine and the Compose plugin using the [official Ubuntu apt repository instructions](https://docs.docker.com/engine/install/ubuntu/#install-using-the-repository). This supports ARM64; do not download an x86-only Compose binary. Then:

```bash
sudo systemctl enable --now docker
sudo usermod -aG docker "$USER"
```

Disconnect and reconnect so the group takes effect. Docker group membership gives administrative access; grant it only to the administrator.

```bash
docker version
docker compose version
git clone YOUR_REPOSITORY_URL imperio-barbaro
cd imperio-barbaro
cp deploy/oracle/.env.example .env.oracle
chmod 600 .env.oracle
openssl rand -hex 32
openssl rand -hex 32
nano .env.oracle
```

Set `DOMAIN=imperiobarbaro.com.br` and `DASHBOARD_DOMAIN=dashboard.imperiobarbaro.com.br` (hostnames only, no `https://` or path), `ACME_EMAIL` to your email, the two distinct generated passwords, clan tags, and API token. Never put credentials in a commit or frontend build. The production stack explicitly uses SQLite and does not inherit a PostgreSQL URL from your development `.env`.

Create a key at the [Clash of Clans developer portal](https://developer.clashofclans.com/) with the VM's public outbound IPv4 in its allowed IP list. Keep the reserved IP attached to this VM; if the outbound address changes, update DNS and replace/reconfigure the API key. Do not use your laptop IP for this server token.

## 3. Deploy and verify

Run from the repository root:

```bash
bash scripts/oracle-compose.sh config -q
bash scripts/oracle-compose.sh up -d --build
bash scripts/oracle-compose.sh ps
bash scripts/oracle-compose.sh logs --tail=100
```

The wrapper always selects the standalone `compose.oracle.yml` and `.env.oracle`. Do not combine it with the development `docker-compose.yml`, which publishes port 2025. Initial Go compilation on 1 OCPU may take several minutes. Images build natively on ARM; no emulation is needed on the A1 VM.

DNS must resolve to the server and ports 80/443 must be reachable for [Caddy automatic HTTPS](https://caddyserver.com/docs/automatic-https). Its certificate state persists in `caddy-data`. Caddy preserves the HTTP Host for the app's same-origin checks and sets HSTS.

```bash
# Expect an HTTP redirect to HTTPS.
curl -I http://YOUR_DOMAIN
# Public homepage: expect 200 without credentials.
curl -I https://imperiobarbaro.com.br
# Private dashboard: expect 401 without credentials.
curl -I https://dashboard.imperiobarbaro.com.br
# Prompts for your APP_PASSWORD; expect {"status":"ok"}.
curl --fail --user leader https://dashboard.imperiobarbaro.com.br/api/health
```

Use your configured username if it differs from `leader`. Do not use `curl -k`: certificate validation should succeed. Open the HTTPS URL, check both clans, unlock contacts, and verify a saved contact survives `bash scripts/oracle-compose.sh restart clanboard`. Health confirms the server responds; the clan screens also verify the external API configuration.

All users share the current application's proxy-facing login attempt limit. Repeated incorrect passwords can temporarily block everyone for up to five minutes. A 502 usually means the app failed to start: inspect app logs and password configuration. Clash 403 errors commonly require checking the token/IP allowlist. TLS failures require checking DNS, OCI ingress and host forwarding/firewall rules.

## 4. Back up and restore

```bash
bash scripts/oracle-backup.sh
```

This pauses the app briefly, archives the complete SQLite volume (including any WAL files), and restarts it even if archiving fails. It requires the app to be running and creates a private `backups/oracle-*.tar.gz`. Run one backup at a time, without concurrent deployment or database operations. Copy completed archives off the VM using `scp`, and keep `.env.oracle` separately in a password manager or other secure backup. Backups contain private contact data.

To schedule a daily backup, run `crontab -e` as the same Docker-enabled administrator and add the following, adjusting the absolute checkout path. The time uses the VM's timezone:

```cron
15 3 * * * cd /home/ubuntu/imperio-barbaro && /usr/bin/flock -n /tmp/imperio-backup.lock /bin/bash scripts/oracle-backup.sh >> /home/ubuntu/imperio-barbaro/backups/backup.log 2>&1
```

Run the manual backup once first to create the directory. Monitor the log and disk space; no archives are automatically deleted. Keep a small rotation (for example seven daily copies) after confirming off-VM copies. Cron does not upload off-VM or notify you automatically.

For disaster recovery, prepare the replacement VM and checkout, restore `.env.oracle`, and build the image. **Restore only an archive you trust, into an empty data volume**:

```bash
bash scripts/oracle-compose.sh build clanboard
# Creates the app container and data volume without starting the app.
bash scripts/oracle-compose.sh create clanboard
bash scripts/oracle-compose.sh run --rm --no-deps -T --entrypoint sh clanboard -c \
  'test -z "$(ls -A /data)" || { echo "Data volume is not empty; restore aborted" >&2; exit 1; }; tar -xzf - -C /data' \
  < backups/YOUR_BACKUP.tar.gz
bash scripts/oracle-compose.sh up -d
```

Validate contacts, raid history and the authenticated health endpoint after restoring. If recovering on an existing server, first take a safety backup and use a separate empty recovery volume/stack; do not overwrite the live database. Never use `down -v` or `docker volume prune` on the production data. For migration from an existing Docker SQLite deployment, stop its writer and archive its entire `/data` volume, then follow the empty-volume restore procedure. Existing PostgreSQL data requires a separate migration; this deployment does not import it automatically.

## 5. Updates and rollback

Record the deployed commit and keep a versioned image tag in `.env.oracle` (`APP_IMAGE_TAG`, for example `release-20261013`). Before updating, run a backup and record the old image tag:

```bash
git rev-parse HEAD
bash scripts/oracle-backup.sh
git pull --ff-only
# Edit APP_IMAGE_TAG to a NEW release identifier before building.
nano .env.oracle
bash scripts/oracle-compose.sh build clanboard
bash scripts/oracle-compose.sh up -d --no-build
```

Repeat the health and browser checks. To roll back code, set `APP_IMAGE_TAG` to the retained old tag and run `up -d --no-build`. Database migrations may require restoring the matching backup to a separate volume before rollback. Keep previous images until the new release is verified. `restart` alone does not reload environment changes; use `up -d`.

Periodically update Ubuntu packages and Caddy (`bash scripts/oracle-compose.sh pull caddy` followed by `up -d`), verify backups, and review OCI resource usage. The containers restart after a VM reboot when Docker is enabled. This is a single-VM deployment: upgrades, backups and VM outages can interrupt service.
