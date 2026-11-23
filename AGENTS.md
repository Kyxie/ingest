# ingest

Go camera-card importer and Baidu Netdisk uploader for an N100 Linux host.
Use Docker Compose for persistent media and physical card access.

- `cmd/ingest`: CLI entry point; `internal/ingest`: import, verification, upload, retention, and tests.
- `deploy/`: standalone Compose update units and isolated development configuration.
- `docs/`: configuration, storage/retention behavior, development, and deployment.
- The miniPC deploys through the `share` stack in `../swarm`, which owns host card-mount units and host scripts (`just share-*`). Do not add host-side scripts here.
- Publish one image tagged only `vA.B.C` and `latest`; it contains both ingest and BaiduPCS-Go. Do not add per-role tag suffixes.
- Preserve read-only card access, destination verification, hardlink sharing, and explicit retention gates.
- Keep media on the HDD and manifests/session state on the SSD. Development uses only `data-dev/`.
- Run `just verify` before declaring a change complete; `just prod-build` checks the runtime image.
- Keep `CLAUDE.md` as the single line `@AGENTS.md`. General personal-project rules live in `../dev-rules/AGENTS.md`; do not duplicate them here.
