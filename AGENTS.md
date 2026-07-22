# Runnarr Agent Guide

This file defines the default operating rules for coding agents working in this repository. Follow it unless the user explicitly gives different instructions.

## Project Context

- Runnarr is a self-hosted, Dockerized activity and health data hub.
- Backend code is Go under `cmd/` and `internal/app/`.
- Frontend code is React/Vite under `web/`.
- PostgreSQL migrations live in `internal/app/migrations/` and run at startup.
- `docs/PRD.md` is the product source of truth.
- `CHANGELOG.md` tracks user-facing and release-relevant changes.

## Standard Commands

- Backend tests: `go test ./...`
- CI-equivalent backend tests: `go test -race ./...`
- Backend vetting: `go vet ./...`
- Go formatting check: `test -z "$(gofmt -l cmd internal)"`
- Frontend tests: `cd web && npm test`
- Frontend build: `cd web && npm run build`
- Full stack rebuild/restart: `docker compose up --build -d`
- Compose smoke check: `curl -fsS http://localhost:37617/api/session`

Use `GOCACHE=/tmp/runnarr-go-cache` if the default Go cache is not writable in the current environment.

## Workflow Defaults

- Do not commit, push, or open a pull request unless the user explicitly asks.
- Unless explicitly told otherwise, open pull requests against `main`.
- For unrelated new work, fetch the latest `origin/main` and create a fresh branch from it.
- Keep unrelated local/user changes intact. Do not revert or overwrite work you did not make.
- Update `CHANGELOG.md` for user-facing changes and release-relevant fixes.
- Update `docs/PRD.md` when product scope, requirements, or roadmap decisions change.
- Keep PRD-only or product-direction changes in a separate commit when they are not part of the implementation.
- Before asking the user to test, rebuild/restart the Docker Compose stack.
- Do not restart or rebuild while a Garmin sync is running unless the user confirms the sync is complete or explicitly says the restart is safe.
- Once the implementation is complete, do not restart the container again; this can break web access for the user.

## Implementation Guidelines

- Prefer existing project patterns over new abstractions.
- Keep changes scoped to the requested behavior.
- Add a numbered SQL migration for database schema changes.
- Preserve nullable normalized fields and raw Garmin/provider payloads when importing provider data so gaps can be debugged later.
- Missing optional UI values should be omitted or left blank, not rendered as placeholder dashes.
- Runnarr is an operational app: keep UI dense, readable, restrained, and built for inspection rather than marketing.
- For frontend controls, use the existing component/style patterns in `web/src/App.tsx` and `web/src/styles.css`.
- For maps, charts, imports, sync, and health data, prefer correctness and inspectability over decorative presentation.

## Verification Expectations

- Run the smallest meaningful checks for the change.
- For backend behavior, run `go test ./...`; use `go test -race ./...` when touching shared state, sync, storage, or CI-sensitive code.
- For frontend behavior, run `cd web && npm test` when tests exist for the affected logic, and `cd web && npm run build` for TypeScript/UI changes.
- For Docker/runtime changes, run `docker compose up --build -d` and smoke check the app.
- If a check cannot be run, say so clearly in the final response and explain why.
