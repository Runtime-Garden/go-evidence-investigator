#!/bin/sh
set -eu
[ -f compose.yaml ] || { echo 'Run from the Compose stack directory' >&2; exit 2; }
container=$(docker compose ps -q evidence-investigator)
[ -n "$container" ] || { echo 'Container not running' >&2; exit 1; }
umask 077
mkdir -p backups
target=$(mktemp -d "backups/case-state-$(date -u +%Y%m%dT%H%M%SZ)-XXXX")
docker cp "${container}:/state" "$target/state"
chmod -R go-rwx "$target"
echo "Saved private, non-transactional snapshot: $target"
echo 'Copy to encrypted offsite backup and rehearse restore before relying on it.'
