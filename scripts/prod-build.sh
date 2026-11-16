#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

docker build --target importer -t local/ingest:latest-importer .
docker build --target uploader -t local/ingest:latest .
