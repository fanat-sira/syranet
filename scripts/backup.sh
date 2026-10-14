#!/bin/sh
# The backup is a private PostgreSQL custom-format archive, never shell credentials.
set -eu
cd "$(dirname "$0")/.."
umask 077
mkdir -p backups
archive="backups/syranet-$(date -u +%Y%m%dT%H%M%SZ)-$$.dump"
tmp="$archive.partial"
trap 'rm -f "$tmp"' EXIT HUP INT TERM
docker compose exec -T postgres sh -c 'exec pg_dump --format=custom --no-owner --no-acl -U "$POSTGRES_USER" -d "$POSTGRES_DB"' > "$tmp"
# Verify that pg_restore recognizes the archive before publishing it.
docker compose exec -T postgres pg_restore --list < "$tmp" > /dev/null
mv "$tmp" "$archive"
sha256sum "$archive" > "$archive.sha256"
echo "Database backup: $archive"
echo 'Also store .env, configs/local.yaml and TLS material in encrypted off-host storage.'
