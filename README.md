# ingest

A headless Go service for an N100 Linux host: copy camera cards to an HDD, verify every copy, and publish files for File Browser. Upload curated folders to Baidu Netdisk on demand.

The project uses the Go standard library, with no frontend or external database server. The Swarm Share stack files Nikon Z fc originals under separate `raw/` and `jpg/` folders. The owner copies selected files into the share and starts uploads manually. Other DCIM cards use generic import rules. Standalone Compose also supports the existing automatic archive workflow.

## Commands

Install Docker with Compose and `just`. Go runs inside Docker; no host Go installation is required.

| Command | Purpose |
| --- | --- |
| `just` | List available commands |
| `just dev` / `just dev-down` | Start/stop the isolated importer with hot reload |
| `just logs [svc]` | Follow development logs |
| `just verify` | Check formatting, vet, race tests, build, scripts, and Compose |
| `just prod-build` | Build the production image as `local/ingest:latest` |
| `just prod-run` / `just prod-stop` | Start/stop standalone Compose production using `.env` |
| `just prod-logs [svc]` | Follow production logs |
| `just pcs-login` | Authorize the standalone BaiduPCS-Go session interactively |
| `just batch-release BATCH_ID` / `just prune` | Release a batch from sharing / run retention cleanup |
| `just release` / `just minor` / `just major` | Publish a patch/minor/major tag and its commits |
| `just clean` | Remove development containers/caches and local images; preserve media/state |

## Deploy

The miniPC runs ingest in the Swarm `share` stack of the [swarm](https://github.com/Kyxie/swarm) repository. That repository also owns the host-side card mount units and the `just share-*` recipes for Baidu login, project uploads, and card ejection, so the miniPC never needs a checkout of this repository.

Standalone Compose remains available: copy `.env.example` to `.env` and `config.example.json` to `config.json`, set `MEDIA_ROOT` to an existing HDD directory, create SSD state/session directories owned by `INGEST_UID:INGEST_GID`, authorize with `just pcs-login`, then run `just prod-run`. See [deployment](docs/deployment.md) for full setup.

Semantic version tag pushes verify and publish one image for `linux/amd64` and `linux/arm64` to GHCR using `GITHUB_TOKEN`, tagged `vA.B.C` and `latest`. It contains the ingest binary and BaiduPCS-Go, so every command (`scan`, `upload`, `project-upload`, maintenance) uses the same image. The builder also checks `ghcr.io/kyxie/baidupcs-go:latest` every six hours and rebuilds `latest` when its digest changes; release tags are never rebuilt. Each build pins the resolved dependency digest. Make the GHCR package public for anonymous pulls. See [deployment](docs/deployment.md#automatic-updates) for the N100 update timer.

## Details

- [Development and verification](docs/development.md): isolated fixtures, hot reload, image checks, and release behavior.
- [Configuration and data safety](docs/configuration.md): storage layout, selection, verification, and retention.
- [Deployment](docs/deployment.md): standalone Compose environment variables, disk permissions, Baidu login, and card mount requirements.
