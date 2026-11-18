#!/usr/bin/env bash
set -euo pipefail

if (( $# > 1 )); then
  echo 'Usage: just eject [partition]' >&2
  exit 2
fi

root=/run/ingest/cards
mapfile -t mounts < <(findmnt -rn -o TARGET -R "$root" | tail -n +2)
cards=()
for mount in "${mounts[@]}"; do
  if [[ $mount =~ ^/run/ingest/cards/(sd[a-z][0-9]{1,2}|mmcblk[0-9]+p[0-9]+)$ ]]; then
    cards+=("${BASH_REMATCH[1]}")
  fi
done

if (( $# == 1 )); then
  device=$1
  if [[ ! $device =~ ^(sd[a-z][0-9]{1,2}|mmcblk[0-9]+p[0-9]+)$ ]] || ! mountpoint -q "$root/$device"; then
    echo "No mounted ingest card named $device" >&2
    exit 1
  fi
elif (( ${#cards[@]} == 1 )); then
  device=${cards[0]}
elif (( ${#cards[@]} == 0 )); then
  echo "No mounted SD card found under $root" >&2
  exit 1
else
  printf 'Multiple SD cards are mounted: %s\n' "${cards[*]}" >&2
  echo 'Specify one, for example: just eject sdc1' >&2
  exit 1
fi

sudo systemctl stop "ingest-card@$device.service"
if mountpoint -q "$root/$device"; then
  echo "Card is still mounted at $root/$device; do not unplug it." >&2
  exit 1
fi
printf 'Safe to remove %s now.\n' "$device"
