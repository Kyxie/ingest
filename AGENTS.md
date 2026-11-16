# ingest

Go camera-card importer and Baidu Netdisk uploader for an N100 Linux host.
Use Docker Compose for persistent media and physical card access.

- `cmd/ingest`: CLI entry point; `internal/ingest`: import, verification, upload, retention, and tests.
- `deploy/`: optional host card-mount units and isolated development configuration.
- `docs/`: configuration, storage/retention behavior, development, and deployment.
- Preserve read-only card access, destination verification, hardlink sharing, and explicit retention gates.
- Keep media on the HDD and manifests/session state on the SSD. Development uses only `data-dev/`.
- Run `just verify` before declaring a change complete; `just prod-build` checks both runtime images.
- Keep `CLAUDE.md` as the single line `@AGENTS.md`. General personal-project rules live in `../dev-rules/AGENTS.md`; do not duplicate them here.
