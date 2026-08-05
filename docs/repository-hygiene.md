# Repository hygiene

This document describes how Runnarr issues are organized. It is a workflow
guide, not a product requirements document.

## Sources of truth

- `README.md` defines the product boundary and the user-visible scope.
- `docs/ROADMAP.md` defines the order of large projects.
- GitHub issues hold individual work items, decisions, and acceptance notes.
- `CHANGELOG.md` records user-facing and release-relevant changes.
- `AGENTS.md` defines the working rules for coding agents.

Keep these roles separate. Do not create a second product specification in the
repository.

## Labels

Use labels in four groups:

- `type:*`: `bug`, `feature`, `enhancement`, `chore`, or `docs`.
- `priority:*`: `high`, `medium`, or `low`. Use `high` for release blockers,
  data-loss/security problems, or broken core workflows.
- `area:*`: the affected product or engineering area, such as `activity`,
  `planning`, `training-sheet`, `garmin`, `mobile`, `ux`, `deployment`,
  `privacy`, `visual`, or `developer-experience`.
- `scope:*`: `release`, `post-v1`, `v2`, or `deferred` when the issue is tied
  to a release boundary or explicitly postponed.

Every triaged issue should have exactly one `type:*` label, one `priority:*`
label, and at least one `area:*` label. Add a `scope:*` label when the issue
is not part of the current release target. Labels describe the issue; the
roadmap and project status describe ordering and progress.

Pull requests may additionally use operational `visual:<viewport>:<scenario>`
labels from `.github/visual-review-profiles.json`. These labels select bounded
before/after Playwright recordings and are not issue classification labels.
Use no more than two on one pull request.

## Milestones and project order

Use milestones for concrete release or delivery targets, not as a replacement
for labels. The large-project order belongs in `docs/ROADMAP.md` and remains
one project at a time. A project may contain several related issues, but only
the active large project should be treated as in progress. Small, isolated
maintenance fixes may proceed in parallel.

When a project is selected:

1. Review its issues and split unclear requests before implementation.
2. Add acceptance criteria and explicit non-goals to the issues that need
   them.
3. Apply one type, one priority, and the relevant area/scope labels.
4. Assign the delivery milestone only when the target release or delivery
   window is known.
5. Update `docs/ROADMAP.md` when the project is concluded or its order changes.

## Backlog triage

For each new issue, first decide whether it is a bug, feature, enhancement,
chore, or documentation task. Then record the affected area and priority.
Close duplicates with a link to the canonical issue, and close ideas that are
explicitly outside the product boundary only after recording the reason.

Before implementation, an issue should state:

- the user or operational problem;
- the expected behavior or outcome;
- acceptance criteria that can be checked;
- relevant constraints, data/privacy concerns, or migration requirements;
- what is deliberately out of scope.

The issue forms provide a starting structure, but terse phone-created issues
may be refined during triage rather than rejected.
