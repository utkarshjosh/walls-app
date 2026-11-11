#!/usr/bin/env bash
# Put the catalog service on the server, or update it.
#
#   apps/api/deploy/deploy.sh
#
# What it does: builds `walls` for the server, takes a consistent copy of the
# local catalog (apps/api/data), builds the product page (apps/site), uploads
# all three with the compose file and the Caddy config, and (re)starts the
# service. Safe to run again after `walls seed` or `walls ingest`: the server
# ends up serving exactly the local catalog and the page as it is in this folder.
#
# Settings (environment variables):
#   WALLS_DEPLOY_HOST   ssh alias of the server         (default agent-server-oci)
#   WALLS_DOMAIN        the public name                 (default walls.utkarshjoshi.com)
#   WALLS_DEPLOY_DIR    folder in the remote home       (default walls)
#   WALLS_SKIP_SITE     set to 1 to leave the product page on the server as it is
set -euo pipefail

HOST="${WALLS_DEPLOY_HOST:-agent-server-oci}"
DOMAIN="${WALLS_DOMAIN:-walls.utkarshjoshi.com}"
DIR="${WALLS_DEPLOY_DIR:-walls}"

cd "$(dirname "$0")/.."
[ -f data/catalog.db ] || { echo "No catalog here yet. Run: go run ./cmd/walls seed" >&2; exit 1; }
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

echo "1/5  Building walls for the server (linux/arm64)"
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "$work/walls" ./cmd/walls

echo "2/5  Copying the catalog"
# A proper backup, so a server running locally cannot leave the copy half-written.
sqlite3 data/catalog.db ".backup '$work/catalog.db'"
items="$(sqlite3 "$work/catalog.db" 'select count(*) from items')"

site=../site
if [ "${WALLS_SKIP_SITE:-0}" = 1 ] || [ ! -f "$site/package.json" ]; then
  echo "3/5  Leaving the product page as it is on the server"
  site=""
else
  echo "3/5  Building the product page"
  [ -d "$site/node_modules" ] || npm --prefix "$site" ci --no-audit --no-fund
  npm --prefix "$site" run build >/dev/null
  [ -f "$site/dist/index.html" ] || { echo "The page did not build: no $site/dist/index.html" >&2; exit 1; }
fi

echo "4/5  Uploading to $HOST:$DIR ($items items)"
ssh "$HOST" "mkdir -p '$DIR/bin' '$DIR/data/objects' '$DIR/site'"
[ -z "$site" ] || rsync -az --delete "$site/dist/" "$HOST:$DIR/site/"
rsync -az --delete data/objects/ "$HOST:$DIR/data/objects/"
rsync -az "$work/catalog.db" "$HOST:$DIR/data/catalog.db.new"
rsync -az "$work/walls" "$HOST:$DIR/bin/walls.new"
rsync -az deploy/compose.yaml deploy/Caddyfile "$HOST:$DIR/"

# The list of people who asked for the build exists only on the server
# (data/records.db there; nothing here replaces it). Keep a copy on this machine
# each time, so losing the server does not lose the list.
mkdir -p data/backups
backup="data/backups/signups-$(date +%Y%m%d-%H%M%S).csv"
if ssh "$HOST" "cd '$DIR' && sudo docker compose exec -T api /app/walls signups" > "$backup" 2>/dev/null && [ -s "$backup" ]; then
  echo "     Saved the sign-up list ($(($(wc -l < "$backup") - 1)) people) to apps/api/$backup"
else
  rm -f "$backup"
  echo "     No sign-up list saved (normal on the first deploy that has sign-ups)"
fi

echo "5/5  Starting the service"
ssh "$HOST" "set -e; cd '$DIR'
  printf 'WALLS_DOMAIN=%s\nWALLS_UID=%s\nWALLS_GID=%s\n' '$DOMAIN' \"\$(id -u)\" \"\$(id -g)\" > .env
  sudo docker compose stop api >/dev/null 2>&1 || true
  mv data/catalog.db.new data/catalog.db
  rm -f data/catalog.db-wal data/catalog.db-shm
  mv bin/walls.new bin/walls && chmod 755 bin/walls
  sudo docker compose up -d --remove-orphans
  # Caddy only reads its config when it starts, and an uploaded config is a new
  # file the running container does not see: restart it so changes take effect.
  sudo docker compose restart web >/dev/null
  sleep 3
  sudo docker compose ps --format 'table {{.Service}}\t{{.Status}}'"

echo
echo "Done. Check it:  curl https://$DOMAIN/v1/health   and open https://$DOMAIN/"
