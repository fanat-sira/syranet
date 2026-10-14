#!/bin/sh
set -eu

echo "Deploying Syranet VPN"

if ! systemctl is-active --quiet sing-box; then
    echo "sing-box is not running"
    exit 1
fi

backup_path="/etc/sing-box/config.json.backup.$(date +%Y%m%d_%H%M%S)"
cp /etc/sing-box/config.json "$backup_path"
echo "Backup created at $backup_path"

docker compose build
docker compose up -d

sleep 5

if ! docker compose ps --status running syranet-vpn | grep -q "syranet-vpn"; then
    echo "Syranet VPN failed to start"
    docker compose logs syranet-vpn
    exit 1
fi

echo "Syranet VPN deployed successfully"
echo "Logs: docker compose logs -f syranet-vpn"
