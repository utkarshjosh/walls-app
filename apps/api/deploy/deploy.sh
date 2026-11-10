#!/usr/bin/env bash
# Put the catalog service on the server, or update it.
#
#   apps/api/deploy/deploy.sh
#
# What it does: builds `walls` for the server, takes a consistent copy of the
# local catalog (apps/api/data), uploads both with the compose file and the
# Caddy config, and (re)starts the service. Safe to run again after `walls seed`
# or `walls ingest`: the server ends up serving exactly the local catalog.
#
# Settings (environment variables):
#   WALLS_DEPLOY_HOST   ssh alias of the server         (default agent-server-oci)
#   WALLS_DOMAIN        the public name                 (default walls.utkarshjoshi.com)
#   WALLS_DEPLOY_DIR    folder in the remote home       (default walls)
set -euo pipefail

HOST="${WALLS_DEPLOY_HOST:-agent-server-oci}"
DOMAIN="${WALLS_DOMAIN:-walls.utkarshjoshi.com}"
DIR="${WALLS_DEPLOY_DIR:-walls}"

cd "$(dirname "$0")/.."
[ -f data/catalog.db ] || { echo "No catalog here yet. Run: go run ./cmd/walls seed" >&2; exit 1; }
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

echo "1/4  Building walls for the server (linux/arm64)"
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "$work/walls" ./cmd/walls

echo "2/4  Copying the catalog"
# A proper backup, so a server running locally cannot leave the copy half-written.
sqlite3 data/catalog.db ".backup '$work/catalog.db'"
items="$(sqlite3 "$work/catalog.db" 'select count(*) from items')"

echo "3/4  Uploading to $HOST:$DIR ($items items)"
ssh "$HOST" "mkdir -p '$DIR/bin' '$DIR/data/objects'"
rsync -az --delete data/objects/ "$HOST:$DIR/data/objects/"
rsync -az "$work/catalog.db" "$HOST:$DIR/data/catalog.db.new"
rsync -az "$work/walls" "$HOST:$DIR/bin/walls.new"
rsync -az deploy/compose.yaml deploy/Caddyfile "$HOST:$DIR/"

echo "4/4  Starting the service"
ssh "$HOST" "set -e; cd '$DIR'
  printf 'WALLS_DOMAIN=%s\nWALLS_UID=%s\nWALLS_GID=%s\n' '$DOMAIN' \"\$(id -u)\" \"\$(id -g)\" > .env
  sudo docker compose stop api >/dev/null 2>&1 || true
  mv data/catalog.db.new data/catalog.db
  rm -f data/catalog.db-wal data/catalog.db-shm
  mv bin/walls.new bin/walls && chmod 755 bin/walls
  sudo docker compose up -d --remove-orphans
  sleep 3
  sudo docker compose ps --format 'table {{.Service}}\t{{.Status}}'"

echo
echo "Done. Check it:  curl https://$DOMAIN/v1/health"
