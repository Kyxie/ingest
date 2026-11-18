#!/usr/bin/env bash
set -euo pipefail

root=$(mktemp -d)
trap 'rm -rf "$root"' EXIT
mkdir -p "$root/bin"
cat > "$root/bin/findmnt" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' /run/ingest/cards
for device in ${TEST_CARDS:-}; do
  printf '/run/ingest/cards/%s\n' "$device"
done
EOF
cat > "$root/bin/mountpoint" <<'EOF'
#!/usr/bin/env bash
case "$2" in
  /run/ingest/cards/sdc1) [[ ${TEST_CARDS:-} == *sdc1* && ! -e $TEST_LOG ]] ;;
  /run/ingest/cards/sdd1) [[ ${TEST_CARDS:-} == *sdd1* && ! -e $TEST_LOG ]] ;;
  *) exit 1 ;;
esac
EOF
cat > "$root/bin/sudo" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$TEST_LOG"
"$@"
EOF
cat > "$root/bin/systemctl" <<'EOF'
#!/usr/bin/env bash
[[ $1 == stop && $2 == ingest-card@sdc1.service ]]
EOF
chmod +x "$root/bin/"*
export PATH="$root/bin:$PATH" TEST_LOG="$root/commands"

if TEST_CARDS='' bash scripts/eject-card.sh >"$root/out" 2>&1; then
  echo 'Expected no-card rejection' >&2; exit 1
fi
if TEST_CARDS='sdc1 sdd1' bash scripts/eject-card.sh >"$root/out" 2>&1; then
  echo 'Expected multiple-card rejection' >&2; exit 1
fi
if [[ -e $TEST_LOG ]]; then
  echo 'Rejected cases called sudo' >&2; exit 1
fi
TEST_CARDS='sdc1' bash scripts/eject-card.sh >"$root/out"
grep -qx 'systemctl stop ingest-card@sdc1.service' "$TEST_LOG"
grep -q 'Safe to remove' "$root/out"
