#!/usr/bin/env bash
# Put a build of the app on the website.
#
#   apps/api/deploy/release.sh <version> <platform> <file> [notes]
#
# For example, from apps/desktop after `npm run build`:
#   ../api/deploy/release.sh 0.1.0 mac-arm64 src-tauri/target/release/bundle/dmg/Still_0.1.0_aarch64.dmg "The first public beta."
#
# What it does: copies the file to the server and runs `walls release -publish`
# there. That adds the file to the release with that version (starting the
# release if this is its first file) and publishes it, so the website offers
# it within a minute. Running it again for the same version and platform
# replaces the file. It is the admin portal's Releases tab without a browser.
#
# <platform> is mac-universal, mac-arm64, mac-x64, windows-x64, windows-arm64,
# linux-x64 or linux-arm64. [notes] is shown on the website under the version.
#
# Settings (environment variables):
#   WALLS_DEPLOY_HOST   ssh alias of the server         (default agent-server-oci)
#   WALLS_DEPLOY_DIR    folder in the remote home       (default walls)
#   WALLS_DRAFT         set to 1 to add the file without publishing the release
set -euo pipefail

HOST="${WALLS_DEPLOY_HOST:-agent-server-oci}"
DIR="${WALLS_DEPLOY_DIR:-walls}"

[ $# -ge 3 ] && [ $# -le 4 ] || { echo "usage: $0 <version> <platform> <file> [notes]" >&2; exit 2; }
version="$1" platform="$2" file="$3" notes="${4:-}"
[ -f "$file" ] || { echo "No such file: $file" >&2; exit 1; }
name="$(basename "$file")"

echo "1/2  Copying $name to $HOST"
ssh "$HOST" "mkdir -p '$DIR/data/incoming'"
rsync -az "$file" "$HOST:$DIR/data/incoming/$name"

echo "2/2  Adding it to release $version"
flags=()
[ "${WALLS_DRAFT:-0}" = 1 ] || flags+=(-publish)
[ -z "$notes" ] || flags+=(-notes "$notes")
# Each word is quoted for the server's shell, so notes with spaces or quotes
# arrive as they were written. The service reads the file from its own data
# folder, which is where the copy went.
remote="$(printf '%q ' /app/walls release ${flags[@]+"${flags[@]}"} "$version" "$platform" "/data/incoming/$name")"
ssh "$HOST" "cd '$DIR' && sudo docker compose exec -T api $remote; status=\$?; rm -rf data/incoming; exit \$status"
