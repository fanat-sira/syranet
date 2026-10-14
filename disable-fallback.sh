#!/bin/sh
set -eu

echo "This will remove fallback credentials from .env"
echo "Make sure all users already switched to new keys"
printf "Continue? (yes/no): "
read -r confirm

if [ "$confirm" != "yes" ]; then
    echo "Aborted"
    exit 1
fi

if [ ! -f .env ]; then
    echo ".env file not found"
    exit 1
fi

cp .env ".env.backup.$(date +%Y%m%d_%H%M%S)"

tmp_file="$(mktemp)"
grep -v '^FALLBACK_' .env > "$tmp_file" || true
mv "$tmp_file" .env

docker compose up -d --force-recreate syranet-vpn

echo "Fallback credentials removed and syranet-vpn recreated"
