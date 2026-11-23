#!/usr/bin/env bash
set -euo pipefail

for image in local/ingest:latest local/ingest:verify; do
  if docker image inspect "$image" >/dev/null 2>&1; then
    docker image rm "$image"
  fi
done
