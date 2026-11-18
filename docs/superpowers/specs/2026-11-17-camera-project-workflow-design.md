# Camera import, project curation, and manual upload

## Workflow

Ingest copies a mounted camera card to the HDD automatically. The owner browses the imported media in File Browser, copies selected files into a named project, and can rename files and create subdirectories in that project. A command on the miniPC uploads one selected project to the configured Baidu Netdisk directory for the device chosen by the owner. The Share stack runs the importer and File Browser without a background uploader.

```text
share/
├── originals/<batch>/DCIM/102NZ_FC/
│   ├── DSC_3651.NEF
│   ├── DSC_3651.JPG
│   └── NC_FLLST.DAT
└── shared/
    ├── imports/20261117222542/
    │   ├── raw/DSC_3651.NEF
    │   └── jpg/DSC_3651.JPG
    └── projects/2026 青岛/
        └── raw/DSC_3651.NEF
```

## Import and device rules

- Keep the card mount read only. Preserve the original DCIM paths and filenames in the verified HDD copy. Keep the existing manifest, retry, and deduplication behavior. Publish the browseable files only after the HDD copy passes verification.
- Name the browseable import directory using the configured local timezone and the time the importer first observes the mounted card. Persist the chosen name with the batch so retries use the same path. Resolve same-second collisions without overwriting. Keep the internal batch ID stable and separate from the display name.
- For a recognized Nikon Z fc card, place `.NEF` in `raw/` and `.JPG` or `.JPEG` in `jpg/`, matching extensions without regard to case. Hide the camera's DCIM folders in the browseable view. Leave Nikon housekeeping files in `originals/`. Preserve both files in a RAW and JPEG pair. Resolve filename collisions caused by removing DCIM folders without overwriting either file.
- Keep card recognition and classification separate from copying, hash verification, and manifest handling. Use the observed Nikon marker and DCIM layout to recognize the camera. The importer's preparation and verification apply to all devices. Preserve the existing generic behavior for unrecognized cards. Add DJI recognition and media rules after inspecting a real DJI card.
- Keep existing batches readable without moving or renaming their originals. The Nikon test batch can gain a classified view from the verified HDD copies without rereading the SD card. Associate new managed paths with their batch so cleanup can account for both views.

## File Browser

Serve `shared/`. Keep managed import hardlinks read only through File Browser, including for the owner, so writing through a share path cannot change an original. Let the owner create and manage folders under `shared/projects/` and copy selected files there using File Browser. Project copies are independent of the original and may contain subdirectories of any depth. Preserve the existing `shared/manual/` directory. Grant friends browse and download permissions for their assigned scope. Keep `originals/`, state, and the Baidu account directory outside the file source.

## Manual upload

- Expose a host command with an explicit device and local project name, such as `just pcs-upload zfc "2026 青岛"`. Provide a host entry point that works with Portainer Git deployment without a permanent checkout. Reuse the existing BaiduPCS-Go session on the SSD.
- Resolve the project inside `shared/projects/`. Reject path traversal, symlinks, and special files. Recursively upload the regular files the owner selected. Take no action on other projects or on files in `originals/` and `shared/imports/`.
- Preserve relative paths under the project. `2026 青岛/raw/DSC_3651.NEF` keeps its `raw/` folder in Baidu Netdisk. Flat projects remain flat. Upload both RAW and JPEG when the owner puts both in the project.
- Resolve `zfc` through the configured device-to-remote-folder mapping. Obtain the exact Nikon destination from BaiduPCS-Go, including spaces and punctuation. Fail before uploading if the device has no configured destination. Allow DJI routing to be configured when its destination is known.
- Show the project, destination, and file list before sending data. Require explicit agreement to use an existing remote project directory. For an existing file of identical content, skip the transfer. For a same-name file with different or unverifiable content, stop and report the conflict. Verify new uploads by readback and record results per file so a retry can use prior results. Never delete remote files as part of this command.
- Keep project upload results independent of the import archive receipts. Project curation can omit files or rename them, so completion of a project upload does not make a managed import eligible for pruning. Keep automatic pruning disabled.

## Acceptance checks

- A Nikon card with one RAW and JPEG pair produces verified HDD copies, read-only imports under `raw/` and `jpg/`, and no DAT file in the browseable import view. Retry and legacy-batch checks preserve earlier data.
- The owner can copy files from several import batches into a project and create a nested folder through File Browser. Friends can download only from the scope they were granted. Neither can edit a managed import through File Browser.
- A mock remote receives the same relative paths for flat and nested projects. An unknown device, a symlink, or a remote content conflict blocks the affected upload. A verified file can be skipped on retry.
- On the miniPC, one selected project passes a real Baidu upload and readback. Importing another card alone creates no new Baidu files.
