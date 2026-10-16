#!/usr/bin/env bash
set -euo pipefail
umask 077
cd "$(dirname "$0")/.."
mkdir -p .deploy
exec 9>.deploy/operation.lock
flock -n 9 || { echo 'Another backup or deployment is running.' >&2; exit 1; }
compose() { bash scripts/oracle-compose.sh "$@"; }
if [[ -z "$(compose ps --status running -q clanboard)" ]]; then
  echo 'Start clanboard before running this backup.' >&2
  exit 1
fi
mkdir -p backups
backup="$(mktemp "backups/oracle-$(date -u +%Y%m%dT%H%M%SZ)-XXXXXX")"
cleanup() {
  result=$?
  trap - EXIT
  if ! compose start clanboard; then result=1; fi
  if [[ $result -ne 0 ]]; then
    echo "Backup failed or app restart failed; inspect $backup and container status." >&2
  fi
  exit "$result"
}
trap cleanup EXIT
compose stop clanboard
compose run --rm --no-deps -T --entrypoint tar clanboard -C /data -czf - . > "$backup"
tar -tzf "$backup" >/dev/null
mv "$backup" "$backup.tar.gz"
echo "Backup saved: $backup.tar.gz (copy it off the VM)."
