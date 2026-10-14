#!/bin/sh
set -eu
cd "$(dirname "$0")"
python3 scripts/setup.py --check
docker compose config --quiet
docker compose build
docker compose run --rm --no-deps manager -check
docker compose up -d --wait --wait-timeout 180
