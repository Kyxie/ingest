# Camera Project Workflow Implementation Plan

**Goal:** Import Nikon cards automatically and upload a selected File Browser project on demand with its relative paths intact.

**Architecture:** Keep verified originals and legacy archive state in place. Persist a separate mapping for the Nikon browse view and use an independent manual upload command and receipt. Swarm runs the importer and File Browser. A one-shot uploader reuses the host's Baidu session.

**Tech stack:** Go, Docker Swarm, File Browser Quantum, BaiduPCS-Go.

- [x] Test legacy generic imports, a Nikon RAW/JPEG/DAT layout, collision handling, and publishing an existing Nikon batch. Record the browse directory and per-file published paths in the batch manifest. Reuse the mapping for retries and retention. Load the configured timezone in the importer image.
- [x] Test flat and nested files with a fake PCS program. Add `project-upload` with `--device`, `--project`, `--apply`, and `--merge`. Validate the project and route, preview before querying or writing remote files, use the PCS `skip` policy, and verify uploads by SHA-256 readback. Keep project receipts outside the batch receipt namespace.
- [x] Provide a miniPC command and a `just` recipe backed by the same wrapper. Keep exact remote destinations configurable. Remove the scheduled uploader from the Share stack and make only the projects directory writable. Verify the stack config and document release and deployment order.
- [x] Run `just verify`, `just prod-build`, and `docker stack config` for the Share stack.
- [ ] After the owner publishes the images and updates Share, verify a real Nikon import, File Browser curation, and Baidu upload on the miniPC.
