# Development and verification

Run `just dev` to build the development image and start Air hot reload. Go and Air run inside Docker. Module/build caches use named volumes; the Compose project is `ingest-dev`, separate from production `ingest`.

Development mounts only `data-dev/cards`, `data-dev/media`, and `data-dev/state`. It does not start an uploader or use production credentials. Air does not load `.env`, and its watcher excludes runtime data. Put synthetic fixtures under `data-dev/cards/<card>/DCIM/`; they are imported to `data-dev/media` after the stability interval. The development configuration disables physical mount requirements and automatic pruning. Runtime directories are ignored by Git and Docker builds.

Use `just logs` to follow imports and `just dev-down` to stop. `just clean` removes development containers, cache volumes, and locally built images; it preserves all bind-mounted media, manifests, and session files.

## Quality gate

`just verify` validates shell syntax and both Compose files, runs the release-script tests in temporary Git repositories with local file remotes, and builds the Dockerfile's `verify` target:

1. Check Go formatting without rewriting files.
2. Run `go vet ./...`.
3. Run `go test -race ./...` with the container's C compiler.
4. Build a stripped static Go binary.

The verification image never mounts local media or account credentials. Go tests use temporary files and a fake PCS executable. They cover selection, hardlinks, incremental imports, interrupted copies, corrupt destinations, symlink/path rejection, future camera clocks, remote verification failures, and retention gates. They do not access a real Baidu account or validate physical SD-card insertion/ejection. CI runs the same `just verify` command before publishing.

`just prod-build` builds the scratch importer as `local/ingest:latest-importer` and the Alpine uploader as `local/ingest:latest`. To test these images with production Compose, explicitly set `IMAGE_REPOSITORY=local/ingest` in `.env`; `just prod-run` accesses the production paths configured there. Use disposable paths for local runtime tests.

Local builds default to `ghcr.io/kyxie/baidupcs-go:v4.0.2`. Override the dependency with `PCS_IMAGE=ghcr.io/kyxie/baidupcs-go@sha256:<digest> just prod-build`. The uploader copies the static executable from that image; it does not clone upstream source. The dependency must provide both target architectures before publishing ingest. CI resolves `latest` to a digest and validates both architectures before building.

## Releases

Releasing is an explicit user action: `just release` (patch), `just minor`, or `just major`. The script checks `Kyxie <kyriexie@outlook.com>`, a clean working tree, an attached branch, and that the branch is not behind or diverged from origin. It fetches remote tags before calculating the version; without tags, a patch release starts at `v0.0.1`.

It creates a local tag and atomically pushes both branch HEAD and the tag to origin. A failed push removes the local tag. No version is changed in CI. Tag builds publish both services with release tags. Scheduled dependency builds and manual retries from `main` publish only uploader `latest` aliases. Ordinary commits and pull requests do not run the builder workflow, so run `just verify` locally before requesting a release.
