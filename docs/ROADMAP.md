# Runnarr Roadmap

This file is the canonical order for the large projects planned after the
1.0.0 release. Work on one large project at a time. Small, genuinely isolated
maintenance fixes may continue in parallel, but must not expand into a second
project.

When a project is concluded, update this file before starting the next one.
When asked “what’s next?”, inspect this roadmap together with the current
issues, pull requests, and branch state.

## Order

### 1. 1.0.0 release — complete (2026-07-30)

- [Issue #76](https://github.com/bznein/Runnarr/issues/76)
- [PR #155](https://github.com/bznein/Runnarr/pull/155)
- [PR #158](https://github.com/bznein/Runnarr/pull/158)

Completed the agreed v1 scope and release checks.

### 2. Repository hygiene — complete (2026-07-27)

- [PR #196](https://github.com/bznein/Runnarr/pull/196)
- Added the issue labels, milestones, issue templates, pull request template,
  and backlog-triage guide.
- Classified the open backlog and kept `README.md` as the product boundary;
  no separate PRD was added.
- GitHub Projects were evaluated and intentionally left unused for now; the
  roadmap, labels, and milestones provide the current ordering.

### 3. Reusable GitHub agent orchestration

Build the repository-agnostic issue-to-plan-to-PR workflow in a separate
repository. Runnarr should be only a configured adopter. The reusable core
must support clarification questions, explicit plan approval, isolated
implementation, PR review follow-up, and human-controlled merge/deployment.

Runnarr-specific staging work from [Issue #179](https://github.com/bznein/Runnarr/issues/179)
is a separate integration concern and must not expand the reusable core.

### 4. Planned-run and training-sheet workflow

- Current `issue-169-planned-run-candidates` branch
- [Issue #180](https://github.com/bznein/Runnarr/issues/180)
- [Issue #183](https://github.com/bznein/Runnarr/issues/183)
- [Issue #184](https://github.com/bznein/Runnarr/issues/184)
- [Issue #189](https://github.com/bznein/Runnarr/issues/189)
- [Issue #148](https://github.com/bznein/Runnarr/issues/148)
- [Issue #151](https://github.com/bznein/Runnarr/issues/151)

### 5. Garmin workout scheduling

- [Issue #170](https://github.com/bznein/Runnarr/issues/170)
- Active implementation: parsing, authoring, calendar scheduling, Garmin
  ownership/reconciliation, activity auto-matching, and the offline testbed.

### 6. User notifications — next

- [Issue #174](https://github.com/bznein/Runnarr/issues/174)
- The first required notification is a user-visible event when a workout is
  newly generated or scheduled. This is the next product issue after #170.

### 7. Course support

- [Issue #102](https://github.com/bznein/Runnarr/issues/102)

### 8. Garmin sync and writeback

- [Issue #181](https://github.com/bznein/Runnarr/issues/181)
- [Issue #187](https://github.com/bznein/Runnarr/issues/187)
- [Issue #49](https://github.com/bznein/Runnarr/issues/49)

### 9. Mobile and activity UX

- [Issue #191](https://github.com/bznein/Runnarr/issues/191)
- [Issue #190](https://github.com/bznein/Runnarr/issues/190)
- [Issue #188](https://github.com/bznein/Runnarr/issues/188)
- [Issue #186](https://github.com/bznein/Runnarr/issues/186)
- [Issue #172](https://github.com/bznein/Runnarr/issues/172)

### 10. Information architecture

- [Issue #161](https://github.com/bznein/Runnarr/issues/161)
- [Issue #137](https://github.com/bznein/Runnarr/issues/137)
- [Issue #177](https://github.com/bznein/Runnarr/issues/177)

### 11. Self-hosting and privacy

- [Issue #160](https://github.com/bznein/Runnarr/issues/160)
- [Issue #121](https://github.com/bznein/Runnarr/issues/121)

### 12. Deferred product extensions

- Pace bands: [Issue #133](https://github.com/bznein/Runnarr/issues/133)
- Themes: [Issue #178](https://github.com/bznein/Runnarr/issues/178)
- Branding/logo: [Issue #185](https://github.com/bznein/Runnarr/issues/185)

## Maintenance lane

Small fixes such as [Issue #182](https://github.com/bznein/Runnarr/issues/182)
may be handled without changing the active large project. A task belongs in
the maintenance lane only when it is bounded, independently testable, and does
not require schema changes, cross-cutting architecture, or a new project
decision.

## Project completion checklist

Before moving to the next project:

- acceptance criteria are met;
- relevant backend, frontend, E2E, and deployment checks pass;
- related issues are closed, split, or explicitly parked;
- remaining risks and follow-up work are recorded;
- activity/training changes state whether Garmin sync, training-sheet
  sync/writeback, or no sync is required;
- this roadmap is updated.
