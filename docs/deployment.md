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
| `latest`, `vA.B.C` | Alpine uploader with TLS certificates, timezone data, and BaiduPCS-Go copied from a resolved personal image digest |
| `latest-uploader`, `vA.B.C-uploader` | Uploader aliases |
| `latest-importer`, `vA.B.C-importer` | Minimal static scratch importer |

Build stages run on the builder platform and cross-compile using `TARGETOS`/`TARGETARCH`. The uploader copies `/usr/local/bin/BaiduPCS-Go` from `ghcr.io/kyxie/baidupcs-go` for its target platform. Publish its multi-platform image from the `actions` repository before the first ingest build. QEMU is configured before buildx for runtime package installation; each target has its own GitHub Actions cache. Images include an OCI source label linking to their repository.

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

## Automatic updates

The `actions` repository checks upstream stable releases daily at 10:00 UTC and builds `ghcr.io/kyxie/baidupcs-go` for amd64 and arm64. Ingest checks that image every six hours at minute 23. A changed digest triggers verification and publication of uploader `latest` and `latest-uploader`; existing release tags and importer images are preserved. Tag releases also use the current dependency digest. The workflow validates both platforms and pins that digest for the entire build, so a concurrent `latest` update cannot mix dependency versions. No cross-repository token is needed; both images must be publicly readable.

Successful builds are recorded in the GitHub Actions cache. A failed build is retried on the next check; eviction of a success marker can cause a harmless rebuild. GitHub schedules may be delayed, and scheduled workflows must be enabled on each repository's default branch. An inactive public repository may have schedules disabled by GitHub. The checker in `actions` records a release only after its image was successfully published.

On the N100, keep `IMAGE_TAG=latest` and install the timer after the initial deployment. The checkout is assumed to live at `/opt/ingest`; if yours differs, use `sudo systemctl edit ingest-update.service` to override `[Service]` / `WorkingDirectory=` before enabling the timer. All bind-mounted media, state, account files, and `.env` stay on the host.

```sh
sudo install -m 0644 deploy/ingest-update.service deploy/ingest-update.timer /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now ingest-update.timer
```

The timer checks five minutes after boot and every fifteen minutes after each run. It pulls images before running Compose; a failed pull leaves the running containers alone. Compose replaces containers whose images changed, so active uploads may be interrupted and retried from their durable receipts. The update does not run a real Baidu login/upload smoke test. The full chain normally takes up to one daily upstream check, one six-hour image check, builds, and one fifteen-minute host check.

Inspect updates with `journalctl -u ingest-update.service` and `systemctl list-timers ingest-update.timer`. Retry an ingest build with **Run workflow** on `ingest-builder` from `main`. Disable host updates with `sudo systemctl disable --now ingest-update.timer`; to pin a release, also set `IMAGE_TAG=vA.B.C` in `.env`. GitHub Actions automatically rebuilds the current default-branch uploader source, so only merge code ready for production.

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

To remove a card, wait for the import to finish, then run `just eject` from a persistent checkout of this repository on the miniPC. It finds the single mounted card and stops its `ingest-card@` service. Only unplug the reader after it prints `Safe to remove`. If more than one card is mounted, use `just eject sdc1` with the partition name shown by `lsblk`; the name can change after reinsertion. A checkout under `/tmp` disappears after reboot, so keep the repository under your home directory if you want to use this recipe later. The command never clears the card or the imported files.

Mount `/run/ingest/cards` into the importer using a **bind with rslave propagation**. The host parent is made rshared by the service; otherwise cards inserted after the container starts remain invisible. Never give the importer privileged mode, host Docker socket, or raw block devices. Only the host helper needs mount privileges. The optional units and hotplug behavior require verification on your real reader; file-based unit tests cannot validate physical hardware.


If you change `CARDS_ROOT`, update host units and helper paths consistently.
