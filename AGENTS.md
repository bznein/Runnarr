# Runnarr Agent Guide

This file defines the default operating rules for coding agents working in this repository. Follow it unless the user explicitly gives different instructions.

## Project Context

- Runnarr is a self-hosted, Dockerized activity and health data hub.
- Backend code is Go under `cmd/` and `internal/app/`.
- Frontend code is React/Vite under `web/`.
- PostgreSQL migrations live in `internal/app/migrations/` and run at startup.
- `README.md` describes the product boundary; GitHub issues record product decisions and release scope.
- `CHANGELOG.md` tracks user-facing and release-relevant changes.
- `docs/repository-hygiene.md` describes the issue, label, milestone, and review workflow.

## Standard Commands

- Backend tests: `go test ./...`
- CI-equivalent backend tests: `go test -race ./...`
- Backend vetting: `go vet ./...`
- Go formatting check: `test -z "$(gofmt -l cmd internal)"`
- Frontend tests: `cd web && npm test`
- Frontend build: `cd web && npm run build`
- Full stack rebuild/restart: `docker compose up --build -d`
- Compose smoke check: `curl -fsS http://localhost:37617/api/session`
- GitHub operations: `gh` is installed and available for PR, check, issue, and Actions queries.
- New features and behavior changes should add or update E2E coverage whenever feasible.
- Local before/after recordings: `VISUAL_PROFILES=visual:desktop:<scenario> make visual-review`
- When asked “what’s next?”, read `docs/ROADMAP.md`, then inspect the current branch, open issues, and open PRs before recommending work. Keep the roadmap’s one-large-project-at-a-time order unless the user explicitly reprioritizes it.

Use `GOCACHE=/tmp/runnarr-go-cache` if the default Go cache is not writable in the current environment.

## Workflow Defaults

- Do not commit, push, or open a pull request unless the user explicitly asks.
- Unless explicitly told otherwise, open pull requests against `main`.
- For unrelated new work, fetch the latest `origin/main` and create a fresh branch from it.
- Before opening a pull request, fetch the latest `origin/main`, update the branch from it, resolve any merge conflicts, run the relevant checks, and only then push/open the PR. If the PR later becomes conflicted, resolve the conflict before pushing again or handing it off.
- After opening or updating a pull request, monitor every required check through completion, including preview deployment checks. Do not hand off while checks are pending. If a check fails, inspect its logs, fix the root cause, push the fix, and repeat monitoring until all required checks pass; report an unavoidable external blocker explicitly.
- For a material user-facing change, apply one or two matching `visual:<viewport>:<scenario>` PR labels from `.github/visual-review-profiles.json`. New screens, navigation or interaction changes, responsive behavior, multi-component layout work, maps/charts, and changed loading, empty, or error states normally qualify. Backend-only work, docs/tests, invisible refactors, copy-only corrections, and genuinely tiny isolated styling fixes normally do not. If uncertain, prefer recording the affected journey.
- If no visual profile exercises a qualifying change, add or extend E2E coverage, give the test a stable visual tag, and register its scenario before requesting visual review. Videos supplement rather than replace assertions, accessibility checks, screenshots, and traces.
- When visual review is requested, do not hand off the PR until the current-head bot comment links the before/after artifacts or an unavoidable workflow blocker has been reported.
- For a release candidate, create a dedicated release branch from the intended `main` commit (for example, `release/v1.0.0`) and cut RC tags from that branch. Commit fixes found during RC validation to the release branch and tag a new RC (`RC2`, etc.); never modify an existing tag. Once the RC is accepted, run the final checks, update release metadata as needed, and tag the final release from the release branch. Keep unrelated post-RC work on separate branches.
- Format pull request descriptions with real Markdown and include a GitHub closing keyword such as `Closes #123` for the issue being addressed. When creating or editing PRs with `gh`, pass a body file or stdin so headings, lists, and blank lines are stored as actual newlines; never submit literal `\n` escapes.
- Keep unrelated local/user changes intact. Do not revert or overwrite work you did not make.
- Update `CHANGELOG.md` for user-facing changes and release-relevant fixes.
- Update `README.md` when the high-level product boundary changes.
- Before asking the user to test, verify the relevant build/runtime checks; do not automatically rebuild or restart a running container.
- Do not restart or rebuild while a Garmin sync is running unless the user confirms the sync is complete or explicitly says the restart is safe.
- Never rebuild or restart any running container after implementation is complete unless the user explicitly authorizes that operation in the current request. This includes public-facing instances; do not infer permission from a need to deploy or from a local Compose workflow.
- After any change affecting activities or training plans, explicitly confirm whether a Garmin sync, training-sheet sync/writeback, or no sync is needed, and briefly explain why.

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
- For every behavior change or bug fix, add or update synthetic/testbed data to cover the implemented case whenever feasible, so the scenario can be exercised in PR previews and staging. If synthetic coverage is infeasible, explicitly state why and identify the automated or manual validation used instead.
- For backend behavior, run `go test ./...`; use `go test -race ./...` when touching shared state, sync, storage, or CI-sensitive code.
- For frontend behavior, run `cd web && npm test` when tests exist for the affected logic, and `cd web && npm run build` for TypeScript/UI changes.
- For Docker/runtime changes, run build and smoke checks without restarting when possible; run `docker compose up --build -d` only after explicit current-request authorization.
- If a check cannot be run, say so clearly in the final response and explain why.
