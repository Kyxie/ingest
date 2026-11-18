set shell := ["bash", "-euc"]
set positional-arguments := true

# List available commands.
default:
    @just --list

# Start the isolated development importer with Go hot reload.
dev:
    bash scripts/dev.sh

# Stop the development importer, preserving local fixtures and media.
dev-down:
    docker compose -f docker-compose.dev.yml down

# Follow development logs, optionally limited to one service.
logs svc="":
    docker compose -f docker-compose.dev.yml logs --tail 100 -f ${1:+"$1"}

# Verify Go formatting, vet, race tests, build, scripts, and Compose.
verify:
    bash scripts/verify.sh

# Build both production images with the local/ingest namespace.
prod-build:
    bash scripts/prod-build.sh

# Start production services using the image repository configured in .env.
prod-run:
    docker compose -f docker-compose.yml up -d

# Stop production services while preserving all media and state.
prod-stop:
    docker compose -f docker-compose.yml down

# Follow production logs, optionally limited to one service.
prod-logs svc="":
    docker compose -f docker-compose.yml logs --tail 100 -f ${1:+"$1"}

# Authorize BaiduPCS-Go interactively and persist its private session on the SSD.
pcs-login:
    docker compose -f docker-compose.yml run --rm --no-deps --entrypoint BaiduPCS-Go uploader login

# Preview or apply a project upload on the Swarm host.
pcs-upload device project *flags:
    bash scripts/host-project.sh "$@"

# Safely unmount the mounted SD card on the miniPC before unplugging it.
eject partition="":
    bash scripts/eject-card.sh ${1:+"$1"}

# Mark a batch as no longer needed by any share before retention cleanup.
batch-release batch:
    docker compose -f docker-compose.yml run --rm --no-deps importer release --config /etc/ingest/config.json --batch "$1"

# Prune only released, expired batches with verified remote receipts.
prune:
    docker compose -f docker-compose.yml run --rm --no-deps importer prune --config /etc/ingest/config.json

# Release a patch with the verified personal Git identity.
release:
    bash scripts/release.sh patch

# Release a minor version with the verified personal Git identity.
minor:
    bash scripts/release.sh minor

# Release a major version with the verified personal Git identity.
major:
    bash scripts/release.sh major

# Remove development containers, caches, and locally built images; retain media.
clean:
    docker compose -f docker-compose.dev.yml down --volumes --rmi local
    bash scripts/clean.sh
