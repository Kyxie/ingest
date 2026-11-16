#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

mkdir -p data-dev/cards data-dev/media data-dev/state
docker compose -f docker-compose.dev.yml up -d --build
