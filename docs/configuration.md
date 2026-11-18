# Configuration and data safety

Use the container paths in `config.example.json` with production Compose; configure host paths in `.env`. Development uses its own configuration under `deploy/`. File Browser is an existing external service, not included here.

## Data layout

With `target=/media` and `state=/state`:

- `/media/originals/<batch>/DCIM/...`: all regular DCIM files, unchanged, with original names and modification times.
- `/media/shared/imports/<batch>/DCIM/...`: hardlinks to JPEG/PNG/MP4/MOV originals for File Browser.
- For a Nikon Z fc card, new browseable views use `/media/shared/imports/<local timestamp>/{raw,jpg}/`. The verified originals still use their camera paths. Recognized Nikon housekeeping files stay with the originals.
- `/media/shared/projects/`: independently editable copies selected for manual upload. Projects can contain subdirectories.
- `/media/shared/manual/`: files you add yourself; never uploaded or pruned by ingest.
- `/media/.verify/`: temporary remote download verification on the HDD.
- `/state/batches/*.json`: original inventory, SHA-256 hashes, selection flags and completion state.
- `/state/receipts/*.json`: per-file upload and remote verification state.

Keep originals and managed share links on **one filesystem**. Do not edit or rename managed batch directories: editing a hardlink also edits the original. Use independent copies in `shared/projects` to make changes. File Browser may not play every HEVC camera video in every browser; downloading the original still works.

## Configuration

Copy `config.example.json` to `config.json`. With the supplied Compose, keep `sources=/cards`, `target=/media`, and `state=/state`; change their host paths in `.env`. For standalone CLI use, set three absolute, non-overlapping paths without symlink components. `sources` contains one immediate subdirectory per card; each must contain DCIM. `require_mount=true` also requires that subdirectory be an actual mount. Set it false for normal source folders or local tests. Destination/state are created if needed, but Compose bind roots must already exist. On a host using an external HDD, verify that it is mounted before starting the service.

The daemon samples file names, sizes and mtimes until unchanged for `stable_seconds` of **host elapsed time**. On recognized Nikon cards, `import_timezone` controls the browse view timestamp, based on when the daemon first observed the card. The default is UTC. A wrong/future camera date does not stop import. The daemon hashes the source, copies to temporary files, fsyncs, reads back the destination, and publishes without overwriting an existing file. Source symlinks and special files are refused. An interrupted batch resumes from its manifest; mismatched existing destinations cause an error, never an overwrite. Unchanged mounted cards are checked cheaply; reinsertion or process restart performs fresh content checks. Records remain after pruning to prevent old card contents being imported again. Cards must be removed long enough for one poll cycle if their content changes without changing names/sizes/mtimes.

The standalone archive worker selects files by extension, case insensitive:

- NEF/DNG/other configured RAW types are uploaded.
- Matching JPEGs in the same relative folder are omitted from cloud archive if a RAW exists. Their camera-rendered look is **not** separately archived. JPEG-only shots are uploaded.
- MP4/MOV and accompanying audio/unknown files are uploaded; configured LRF/THM proxies are not.
- All DCIM files are retained locally until eligible for cleanup. Files outside DCIM are not imported. RAW/JPEG pairing does not combine across folders or insertions.

Add RAW/proxy extensions to support another device. Future formats are retained and archived rather than silently discarded. Device names, card labels and EXIF are not required. The import target must have room for a full card plus one largest remote verification file; disk-full errors preserve the card and any already verified destination files.

## Manual project uploads

Use `project-upload --config PATH --device DEVICE --project NAME` to preview a project under `shared/projects`. Configure `project_remotes` as a map of device keys to exact, existing Baidu Netdisk folders. With `--apply`, the uploader copies the selected project's regular files to the remote device folder and preserves their relative paths. Supply `--merge` if that project directory already exists remotely. Existing files with matching bytes are skipped. A conflict halts the upload without overwriting existing files. New uploads are downloaded back and checked by SHA-256. Reserve HDD scratch space for a staged file and its downloaded copy. Project receipts are kept in `state/project-receipts/` independently of the automatic archive receipts.

To add the classified Nikon view to an earlier batch, run `publish-nikon --config PATH --batch ID`. The command verifies the HDD originals before publishing additional links and retains the old paths. It uses the existing batch creation time for its label.

## Automatic archive upload verification

`upload` requires BaiduPCS-Go on PATH. It uses argument arrays, not a shell, and uploads files independently with `--policy overwrite --norapid` into the batch-specific remote path. Do not manually place files in that remote archive namespace. Failed items retry on the next poll. BaiduPCS-Go currently cannot resume an interrupted single-file upload. No unbounded child output or credential output is retained in ingest logs.

By default, every uploaded file is downloaded into the HDD scratch directory and checked against the recorded size and SHA-256. This consumes additional bandwidth, but validates actual remote bytes and does not rely on Baidu's multipart MD5. `verify_download=false` disables the extra download; such uploads **never qualify for pruning**. After a crash, leftover `.ingest-*`, `.state-*` or `.verify/readback-*` files can be removed while both workers are stopped; they are never considered completed media. Downloads must actually work on your account for receipts to become verified.

## Retention

`auto_prune=false` by default. There is no File Browser API integration or automatic detection of active share links. After removing/expiring all public links to a batch, explicitly release it:

```sh
just batch-release BATCH_ID
just prune
```

Prune requires all three: batch older than `retention_days` (90 by default), explicit release, and verified remote receipts for every archive-selected file. It checks local files for modifications, removes only tracked paths, and keeps manifests/tombstones. Empty directories and files added outside the inventory remain. Expiring a File Browser link alone never releases a batch. Once released, do not create new links to it. Setting `auto_prune=true` enables the same checks each successful scan cycle.

