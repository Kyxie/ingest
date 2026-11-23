#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

build_args=()
if [[ -n "${PCS_IMAGE:-}" ]]; then
  build_args+=(--build-arg "PCS_IMAGE=$PCS_IMAGE")
fi
docker build --target runtime "${build_args[@]}" -t local/ingest:latest .
