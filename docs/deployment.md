# N100 deployment

The N100 runs the importer and uploader with Docker Compose. Keep configuration, manifests, and generated BaiduPCS-Go account state on the SSD; mount media and verification scratch space from the HDD.

## Configuration and permissions

```sh
cp .env.example .env
cp config.example.json config.json
mkdir -p data/state data/pcs
```

Edit `.env`: set `MEDIA_ROOT` to an existing directory on the mounted HDD. `IMAGE_REPOSITORY` defaults to `ghcr.io/kyxie/ingest`; use a lowercase owner/project for a fork. `STATE_ROOT` and `PCS_CONFIG_ROOT` default to SSD directories under `data/`; `CONFIG_PATH` defaults to `./config.json`, and `CARDS_ROOT` defaults to `/run/ingest/cards`. `IMAGE_TAG` defaults to `latest`; use `vA.B.C` to pin a release. Container paths in `config.json` remain `/cards`, `/media`, and `/state`.

Both services run as `INGEST_UID:INGEST_GID` (1000:1000 by default). Media, state, and PCS configuration directories must be writable by that identity; config and cards must be readable. The optional mount helper uses UID/GID 1000, so adjust its options if you change the identity. Set directory ownership explicitly on the host before starting. Both services have memory limits and `restart: unless-stopped`.

Compose refuses missing bind paths instead of creating directories over a missing HDD mount. Verify the HDD is mounted before starting: an existing empty mount directory alone does not prove the disk is present. `just prod-run` does not initialize host paths or install mount units.

`.env`, `config.json`, media, and account state are ignored by Git and excluded from image builds. Keep manually managed secrets in `.env` or GitHub Secrets. BaiduPCS-Go generates its private `pcs_config.json` session cache under `PCS_CONFIG_ROOT`; restrict that directory to the service identity and never commit or share it. Credentials must not go in `config.json`, public logs, or screenshots.

## Images and authorization

GHCR publication uses the workflow's built-in `GITHUB_TOKEN` with `packages: write`; no registry secret or Docker Hub credentials are needed. GHCR packages are private on first publication. Make the package public if the N100 should pull without signing in; otherwise sign in on the N100 with a GitHub token having `read:packages`.

Run `just verify`, commit the intended changes, then explicitly release with `just release`, `just minor`, or `just major`. The tag workflow verifies the project and builds `linux/amd64,linux/arm64` images:

| Tag under `ghcr.io/<github-user>/<project>` | Runtime |
| --- | --- |
| `latest`, `vA.B.C` | Alpine uploader with TLS certificates, timezone data, and pinned BaiduPCS-Go v4.0.2 |
| `latest-uploader`, `vA.B.C-uploader` | Uploader aliases |
| `latest-importer`, `vA.B.C-importer` | Minimal static scratch importer |

Build stages run on the builder platform and cross-compile using `TARGETOS`/`TARGETARCH`. QEMU is configured before buildx for runtime package installation; each target has its own GitHub Actions cache. Images include an OCI source label linking to their repository.

After publishing images and preparing every host directory (including the card mount root below), pull images and authorize BaiduPCS-Go interactively:

```sh
docker compose pull
just pcs-login
just prod-run
```

Enter credentials only in the interactive login prompt. The uploader reuses the generated session under `PCS_CONFIG_ROOT`. Stop the uploader before reauthorizing an existing session. Account compatibility requires testing real upload/download access.

Update the N100 after future releases with:

```sh
docker compose pull && docker compose up -d
```

## Read-only card mounts

Docker does not mount new SD cards automatically. `deploy/` contains optional root-owned host setup for Linux/systemd. Read it before installing on the miniPC:

```sh
sudo install -D -m 0755 deploy/ingest-card-mount /usr/local/libexec/ingest-card-mount
sudo install -m 0644 deploy/ingest-mount-root.service /etc/systemd/system/
sudo install -m 0644 deploy/ingest-card@.service /etc/systemd/system/
sudo install -m 0644 deploy/90-ingest-card.rules /etc/udev/rules.d/
sudo systemctl daemon-reload
sudo systemctl enable --now ingest-mount-root.service
sudo udevadm control --reload-rules
```

Then reinsert a card. The rule selects removable FAT/exFAT partitions. The helper refuses fixed disks, refuses a second mount, and mounts read-only with nodev/nosuid/noexec, UID/GID 1000 and restrictive file masks at `/run/ingest/cards/<device>`. Readers that don't report `removable=1` won't trigger this rule: inspect `udevadm info`/`lsblk` on that host before adapting it. A desktop automounter must not mount the card first. The agent does not install host units during development or verification.

Mount `/run/ingest/cards` into the importer using a **bind with rslave propagation**. The host parent is made rshared by the service; otherwise cards inserted after the container starts remain invisible. Never give the importer privileged mode, host Docker socket, or raw block devices. Only the host helper needs mount privileges. The optional units and hotplug behavior require verification on your real reader; file-based unit tests cannot validate physical hardware.


If you change `CARDS_ROOT`, update host units and helper paths consistently.
