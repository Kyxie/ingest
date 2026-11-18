# Camera import, project curation, and manual upload

## Workflow

Ingest copies a mounted camera card to the HDD automatically. The owner keeps the archive and manually copies selected files from it into a named shared folder. The owner can rename files and create subdirectories in File Browser. A command on the miniPC uploads one selected folder to the configured Baidu Netdisk directory for the device chosen by the owner. The Share stack runs the importer and File Browser without a background uploader.

```text
share/
├── originals/<batch>/
│   ├── raw/DSC_3651.NEF
│   ├── jpg/DSC_3651.JPG
│   └── other/NC_FLLST.DAT
└── shared/
    └── imports/
        └── 2026 青岛/
            └── raw/DSC_3651.NEF
```

## Import and device rules

- Keep the card mount read only and verify the HDD copy. Store the unmodified relative card path and hash in the manifest for deduplication. Record the classified HDD path separately. Keep the batch ID and its existing UTC-based format.
- For a recognized Nikon Z fc card, place `.NEF` in `raw/` and `.JPG` or `.JPEG` in `jpg/`, matching extensions without regard to case. Put Nikon housekeeping files and other DCIM files in `other/`. Preserve both files in a RAW and JPEG pair. Resolve filename collisions caused by removing DCIM folders without overwriting either file.
- Keep card recognition and classification separate from copying, hash verification, and manifest handling. Use the observed Nikon marker and DCIM layout to recognize the camera. The importer's preparation and verification apply to all devices. Preserve the existing generic behavior for unrecognized cards. Add DJI recognition and media rules after inspecting a real DJI card.
- Keep existing batches usable through their legacy paths. Provide an explicit command to organize a completed Nikon archive in place without the SD card. Update the manifest before removing legacy names. Account for the stored path during later deduplication and cleanup.

## File Browser

Serve `shared/`. A fresh Swarm import writes only to `originals/`. The owner copies chosen files into `shared/imports/` and can organize those files in File Browser. Older share hardlinks are detached before File Browser opens for writing. Preserve the existing `shared/manual/` directory. Grant friends browse and download permissions for their assigned scope. Keep `originals/`, state, and the Baidu account directory outside the file source.

## Manual upload

- Expose a host command with an explicit device and local project name, such as `just pcs-upload zfc "2026 青岛"`. Provide a host entry point that works with Portainer Git deployment without a permanent checkout. Reuse the existing BaiduPCS-Go session on the SSD.
- Resolve the selected folder inside `shared/imports/`. Reject path traversal, symlinks, and special files. Recursively upload the regular files in the selected folder. Take no action on neighbouring folders or on `originals/`.
- Preserve relative paths under the project. `2026 青岛/raw/DSC_3651.NEF` keeps its `raw/` folder in Baidu Netdisk. Flat projects remain flat. Upload both RAW and JPEG when the owner puts both in the project.
- Resolve `zfc` through the configured device-to-remote-folder mapping. Obtain the exact Nikon destination from BaiduPCS-Go, including spaces and punctuation. Fail before uploading if the device has no configured destination. Allow DJI routing to be configured when its destination is known.
- Show the project, destination, and file list before sending data. Require explicit agreement to use an existing remote project directory. For an existing file of identical content, skip the transfer. For a same-name file with different or unverifiable content, stop and report the conflict. Verify new uploads by readback and record results per file so a retry can use prior results. Never delete remote files as part of this command.
- Keep project upload results independent of the import archive receipts. Project curation can omit files or rename them, so completion of a project upload does not make a managed import eligible for pruning. Keep automatic pruning disabled.

## Acceptance checks

- A Nikon card with one RAW and JPEG pair produces verified HDD copies under `originals/<batch>/raw` and `originals/<batch>/jpg`. A DAT file goes under `other/`. No shared folder is populated by the scan. Retry and legacy-batch checks preserve earlier data.
- The owner can copy files from several archives into a named shared folder and organize them in File Browser. Friends can download only from the scope they were granted. Editing shared copies does not modify their archived originals.
- A mock remote receives the same relative paths for flat and nested projects. An unknown device, a symlink, or a remote content conflict blocks the affected upload. A verified file can be skipped on retry.
- On the miniPC, one selected project passes a real Baidu upload and readback. Importing another card alone creates no new Baidu files.
