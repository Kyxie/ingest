# ingest

A headless Linux camera-card importer, written in Go. Copies mounted camera cards to disk, verifies the copies, exposes JPEG/video hardlinks for sharing, and queues selected originals for Baidu Netdisk. No frontend, media decoding, date inference, or external database server. Batch directories use **import time in UTC**, not the camera clock.

This is an independent implementation. The related [hktkzyx/ingest](https://github.com/hktkzyx/ingest) project inspired the workflow; no source was copied. First version targets Nikon Z fc and DJI Action 4 cards with a DCIM directory. Any removable card with DCIM is a candidate; it does not prove who owns the card or exactly which model created it. Only plug cards you intend to import into a configured reader.

## Data layout

With `target=/media` and `state=/state`:

- `/media/originals/<batch>/DCIM/...`: all regular DCIM files, unchanged, with original names and modification times.
- `/media/shared/imports/<batch>/DCIM/...`: hardlinks to JPEG/PNG/MP4/MOV originals for File Browser.
- `/media/shared/manual/`: files you add yourself; never uploaded or pruned by ingest.
- `/media/.verify/`: temporary remote download verification on the HDD.
- `/state/batches/*.json`: original inventory, SHA-256 hashes, selection flags and completion state.
- `/state/receipts/*.json`: per-file upload and remote verification state.

Keep originals and shares on **one filesystem**. Do not edit or rename managed batch directories: editing a hardlink also edits the original. Upload/rename personal files under `shared/manual` instead. Removing a share link in File Browser does not delete media. File Browser may not play every HEVC camera video in every browser; downloading the original still works. This does not provide Apple Live Photo playback.

## Configuration

Copy `config.example.json` and change the three absolute, non-overlapping paths. They must not contain symlink components. `sources` contains one immediate subdirectory per card; each must contain DCIM. `require_mount=true` also requires that subdirectory be an actual mount. Set it false for normal source folders or local tests. Destination/state are created if needed. On a host using an external HDD, verify that it is mounted before starting the service.

The daemon samples file names, sizes and mtimes until unchanged for `stable_seconds` of **host elapsed time**. A wrong/future camera date does not stop import. It then hashes the source, copies to temporary files, fsyncs, reads back the destination, and publishes without overwriting an existing file. Source symlinks and special files are refused. An interrupted batch resumes from its manifest; mismatched existing destinations cause an error, never an overwrite. Unchanged mounted cards are checked cheaply; reinsertion or process restart performs fresh content checks. Records remain after pruning to prevent old card contents being imported again. Cards must be removed long enough for one poll cycle if their content changes without changing names/sizes/mtimes.

Archive selection is configurable by extension, case insensitive:

- NEF/DNG/other configured RAW types are uploaded.
- Matching JPEGs in the same relative folder are omitted from cloud archive if a RAW exists. Their camera-rendered look is **not** separately archived. JPEG-only shots are uploaded.
- MP4/MOV and accompanying audio/unknown files are uploaded; configured LRF/THM proxies are not.
- All DCIM files are retained locally until eligible for cleanup. Files outside DCIM are not imported. RAW/JPEG pairing does not combine across folders or insertions.

Add RAW/proxy extensions to support another device. Future formats are retained and archived rather than silently discarded. Device names, card labels and EXIF are not required. The import target must have room for a full card plus one largest remote verification file; disk-full errors preserve the card and any already verified destination files.

## Commands

Build with Go 1.26+ on Linux:

```sh
go test ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o ingest ./cmd/ingest
./ingest scan --config /absolute/config.json --once
./ingest scan --config /absolute/config.json
./ingest upload --config /absolute/config.json
```

`upload` requires BaiduPCS-Go on PATH. It uses argument arrays, not a shell, and uploads files independently with `--policy overwrite --norapid` into the batch-specific remote path. Do not manually place files in that remote archive namespace. Failed items retry on the next poll. BaiduPCS-Go currently cannot resume an interrupted single-file upload. No unbounded child output or credential output is retained in ingest logs.

By default, every uploaded file is downloaded into the HDD scratch directory and checked against the recorded size and SHA-256. This consumes additional bandwidth, but validates actual remote bytes and does not rely on Baidu's multipart MD5. `verify_download=false` disables the extra download; such uploads **never qualify for pruning**. After a crash, leftover `.ingest-*`, `.state-*` or `.verify/readback-*` files can be removed while both workers are stopped; they are never considered completed media. Downloads must actually work on your account for receipts to become verified.

## Retention

`auto_prune=false` by default. There is no File Browser API integration or automatic detection of active share links. After removing/expiring all public links to a batch, explicitly release it:

```sh
./ingest release --config /absolute/config.json --batch BATCH_ID
./ingest prune --config /absolute/config.json
```

Prune requires all three: batch older than `retention_days` (90 by default), explicit release, and verified remote receipts for every archive-selected file. It checks local files for modifications, removes only tracked paths, and keeps manifests/tombstones. Empty directories and files added outside the inventory remain. Expiring a File Browser link alone never releases a batch. Once released, do not create new links to it. Setting `auto_prune=true` enables the same checks each successful scan cycle.

## Docker and host mounting

`Dockerfile` has `importer` (scratch, static Go binary) and `uploader` (Alpine, ingest and pinned BaiduPCS-Go v4.0.2) targets. Tagged GitHub releases publish `ghcr.io/kyxie/ingest:<tag>-importer` and `<tag>-uploader` for amd64. Adapt the image namespace in CI for your fork. These images must be built/published before a Swarm stack can pull them; source code alone is not an available image.

Docker/Swarm does not mount new SD cards automatically. `deploy/` contains optional root-owned host setup for Linux/systemd. Read it before installing on the miniPC:

```sh
sudo install -D -m 0755 deploy/ingest-card-mount /usr/local/libexec/ingest-card-mount
sudo install -m 0644 deploy/ingest-mount-root.service /etc/systemd/system/
sudo install -m 0644 deploy/ingest-card@.service /etc/systemd/system/
sudo install -m 0644 deploy/90-ingest-card.rules /etc/udev/rules.d/
sudo systemctl daemon-reload
sudo systemctl enable --now ingest-mount-root.service
sudo udevadm control --reload-rules
```

Then reinsert a card. The rule selects removable FAT/exFAT partitions. The helper refuses fixed disks, refuses a second mount, and mounts read-only with nodev/nosuid/noexec, UID/GID 1000 and restrictive file masks at `/run/ingest/cards/<device>`. Readers that don't report `removable=1` won't trigger this rule: inspect `udevadm info`/`lsblk` on that host before adapting it. A desktop automounter must not mount the card first. No deployment commands run from this repository.

Mount `/run/ingest/cards` into the importer using a **bind with rslave propagation**. The host parent is made rshared by the service; otherwise cards inserted after the container starts remain invisible. Never give the importer privileged mode, host Docker socket, or raw block devices. Only the host helper needs mount privileges. The optional units and hotplug behavior require verification on your real reader; file-based unit tests cannot validate physical hardware.

## Development and verification

The unit/integration tests use temporary files and a local fake PCS executable. They cover RAW/JPEG selection, hardlinks, repeated/incremental imports, cancelled copies, corrupt destinations, overlap/symlink rejection, future camera clocks, unrelated old files, remote hash failures and retention gates. CI also runs race tests; local environments need a C compiler to run `go test -race`. Tests do not access a real Baidu account or insert/eject a physical SD card.
