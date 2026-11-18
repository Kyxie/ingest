#!/usr/bin/env bash
set -euo pipefail

if (( $# < 2 )); then
  echo "Usage: ingest-project DEVICE 'PROJECT NAME' [--apply] [--merge]" >&2
  exit 2
fi
device=$1
project=$2
shift 2
for option in "$@"; do
  case "$option" in
    --apply|--merge) ;;
    *) printf 'Unknown option: %s\n' "$option" >&2; exit 2 ;;
  esac
done

if ! mountpoint -q /mnt/data; then
  echo "/mnt/data must be mounted before reading the project." >&2
  exit 1
fi
share_root=/mnt/data/hdd/share
share_state=/home/kyxie/data/share
source_image=$(docker service inspect --format '{{.Spec.TaskTemplate.ContainerSpec.Image}}' share_importer)
source_image=${source_image%%@*}
if [[ $source_image != *-importer ]]; then
  echo "The Share importer must use a tagged ingest importer image." >&2
  exit 1
fi
upload_image=${source_image%-importer}-uploader

exec docker run --rm --user 1000:1000 \
  -e BAIDUPCS_GO_CONFIG_DIR=/pcs \
  --mount "type=bind,source=$share_root,target=/media" \
  --mount "type=bind,source=$share_state/state,target=/state" \
  --mount "type=bind,source=$share_state/pcs,target=/pcs" \
  --mount "type=bind,source=$share_state/project-upload.json,target=/etc/ingest/project-upload.json,readonly" \
  "$upload_image" project-upload --config /etc/ingest/project-upload.json \
  --device "$device" --project "$project" "$@"
