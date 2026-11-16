# ingest

A headless Go service for an N100 Linux host: copy camera cards to an HDD, verify every copy, expose JPEG/video hardlinks for sharing, and archive selected originals to Baidu Netdisk.

Deployment uses Docker Compose because physical cards and large media require local storage. The project uses the Go standard library, with no frontend or external database server. Nikon Z fc and DJI Action 4 cards with DCIM are the initial targets. Batch names use import time in UTC.

## Commands

Install Docker with Compose and `just`. Go runs inside Docker; no host Go installation is required.

| Command | Purpose |
| --- | --- |
| `just` | List available commands |
| `just dev` / `just dev-down` | Start/stop the isolated importer with hot reload |
| `just logs [svc]` | Follow development logs |
| `just verify` | Check formatting, vet, race tests, build, scripts, and Compose |
| `just prod-build` | Build both production images as `local/ingest` |
| `just prod-run` / `just prod-stop` | Start/stop production using `.env` |
| `just prod-logs [svc]` | Follow production logs |
| `just pcs-login` | Authorize the production BaiduPCS-Go session interactively |
| `just batch-release BATCH_ID` / `just prune` | Release a batch from sharing / run retention cleanup |
| `just release` / `just minor` / `just major` | Publish a patch/minor/major tag and its commits |
| `just clean` | Remove development containers/caches and local images; preserve media/state |

## Deploy

Copy `.env.example` to `.env` and `config.example.json` to `config.json`. Set `MEDIA_ROOT` to an existing HDD directory; create SSD state/session directories owned by `INGEST_UID:INGEST_GID`. Install the optional read-only card-mount units, authorize with `just pcs-login`, then run `just prod-run`. See [deployment](docs/deployment.md) for full setup.

Semantic version tag pushes verify and publish both `linux/amd64` and `linux/arm64` images to GHCR using `GITHUB_TOKEN`. The builder also checks `ghcr.io/kyxie/baidupcs-go:latest` every six hours and rebuilds the uploader when its digest changes. Each build pins the resolved dependency digest. The uploader uses `latest` / `vA.B.C`; the importer uses `latest-importer` / `vA.B.C-importer`. Automatic dependency updates change only uploader `latest` aliases. Make both GHCR packages public for anonymous pulls. See [deployment](docs/deployment.md#automatic-updates) for the N100 update timer.

## Details

- [Development and verification](docs/development.md): isolated fixtures, hot reload, image checks, and release behavior.
- [Configuration and data safety](docs/configuration.md): storage layout, selection, verification, and retention.
- [Deployment](docs/deployment.md): environment variables, disk permissions, Baidu login, and card mounts.
