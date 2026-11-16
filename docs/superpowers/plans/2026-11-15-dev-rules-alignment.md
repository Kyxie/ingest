# Development Rules Alignment Implementation Plan

> **For agentic workers:** Execute inline in the current session. Do not commit, push, tag, deploy, or install host units without an explicit user request.

**Goal:** Align the existing ingest repository with `../dev-rules/AGENTS.md` while preserving import and retention behavior.

**Update:** The original tag-only publication and source-built BaiduPCS-Go are superseded by the automatic dependency update flow in `docs/deployment.md`. The uploader now consumes the personal multi-platform image.

**Architecture:** Keep the Go CLI and scratch importer/Alpine uploader. Use N100 Docker Compose for physical cards and persistent media, an isolated Docker development environment, and GHCR tag releases with the built-in GITHUB_TOKEN.

**Tech Stack:** Go standard library, Docker Compose, Air, Bash, just, GitHub Actions.

## Global Constraints

- Documents, comments, and rule files are English; user communication is Chinese.
- No new application dependencies or unrelated business refactoring.
- Media lives on the HDD; state and configuration live on the SSD.
- Development and verification never access production media or Baidu credentials.
- Preserve read-only card access, verified copies, hardlinks, and retention gates.

### Task 1: Align development and production configuration

- [x] Compare dev-rules and reference `lanclip`/`telegram` configurations.
- [x] Add just commands, development Compose/Air, production Compose, and `.env.example`.
- [x] Cross-compile runtime binaries; exclude credentials and media from builds.
- [x] Verify Go formatting, vet, race tests, static build, Compose, hot reload, and amd64 runtime images; cross-compile the importer and BaiduPCS-Go for arm64.

### Task 2: Align releases, agent rules, and documentation

- [x] Replace CI with `ingest-builder`, tag validation, GHCR, dual architectures, and GHA caches.
- [x] Adapt the reference release script with identity checks, initial `v0.0.0`, and atomic rollback.
- [x] Test release guards, versions, branch state, and failed-push rollback using local temporary remotes.
- [x] Add short project rules and an import-only `CLAUDE.md`; split the README into topic docs.
- [x] Review the diff, narrow development source mounts, and configure repository-local Git identity. Physical hotplug and real account transfers remain host validation tasks.
