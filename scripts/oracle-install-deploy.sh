#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
checkout="$(pwd -P)"
deploy_user="$(id -un)"
if [[ "$deploy_user" == root || ! "$checkout" =~ ^/[a-zA-Z0-9_./-]+$ ]]; then
  echo 'Run as the Docker-enabled app owner, from a path without spaces.' >&2
  exit 1
fi
python3 scripts/oracle-deploy.py --check
sudo tee /etc/systemd/system/imperio-deploy.service >/dev/null <<EOF
[Unit]
Description=Deploy tested Imperio Barbaro main commits
Wants=network-online.target
After=network-online.target docker.service

[Service]
Type=oneshot
User=$deploy_user
WorkingDirectory=$checkout
Environment=PYTHONUNBUFFERED=1
Environment=GIT_TERMINAL_PROMPT=0
ExecStart=/usr/bin/python3 $checkout/scripts/oracle-deploy.py
TimeoutStartSec=45min
TimeoutStopSec=2min
KillMode=mixed
UMask=0077
EOF
sudo tee /etc/systemd/system/imperio-deploy.timer >/dev/null <<'EOF'
[Unit]
Description=Check for tested Imperio Barbaro releases every five minutes

[Timer]
OnBootSec=2min
OnUnitInactiveSec=5min
RandomizedDelaySec=30s
Unit=imperio-deploy.service

[Install]
WantedBy=timers.target
EOF
sudo systemctl daemon-reload
sudo systemctl enable --now imperio-deploy.timer
systemctl list-timers imperio-deploy.timer --no-pager
echo 'Automatic deployment enabled. Logs: journalctl -u imperio-deploy.service'
