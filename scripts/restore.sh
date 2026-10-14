#!/bin/sh
# Restores into a NEW database only. Never overwrites the configured database.
set -eu
cd "$(dirname "$0")/.."
if [ "$#" -ne 2 ]; then echo 'Usage: scripts/restore.sh <archive.dump> <new_database_name>' >&2; exit 1; fi
archive="$1"
restore_db="$2"
case "$restore_db" in ''|*[!a-zA-Z0-9_]*|[0-9]*) echo 'Invalid new database name' >&2; exit 1;; esac
[ -f "$archive" ] || { echo 'Archive not found' >&2; exit 1; }
docker compose exec -T postgres pg_restore --list < "$archive" > /dev/null
# createdb fails if the name already exists; no DROP, clean or overwrite option.
docker compose exec -T postgres sh -c 'exec createdb -U "$POSTGRES_USER" "$1"' sh "$restore_db"
docker compose exec -T postgres sh -c 'exec pg_restore --exit-on-error --single-transaction --no-owner --no-acl -U "$POSTGRES_USER" -d "$1"' sh "$restore_db" < "$archive"
echo "Restored into $restore_db. Existing database untouched. Verify rows and migrations before any explicit cutover."
echo 'Changing POSTGRES_DB does not initialize roles or replace existing databases.'
