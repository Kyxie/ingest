#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

for script in scripts/*.sh; do
  bash -n "$script"
done
sh -n deploy/ingest-card-mount
bash scripts/test-release.sh

# Never load local production settings or access production mounts during verification.
IMAGE_REPOSITORY=ghcr.io/verify/ingest MEDIA_ROOT=/tmp/ingest-verify-media \
  docker compose --env-file /dev/null -f docker-compose.yml config --quiet
docker compose --env-file /dev/null -f docker-compose.dev.yml config --quiet

docker build --target verify -t local/ingest:verify .
