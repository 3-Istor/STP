# Spec Drum conventions (Codex)

This directory stores versioned feature specifications. Each subdirectory is a ticket.

## Structure

```text
.specs/
  AGENTS.md                 # This file, conventions for specs
  <ticket-id>/
    spec.md                 # Context, research, requirements, tasks and checks
    state.md                # Status, progress, task log, decisions, blockers and reports
```

## Lifecycle

`draft` → `in-progress` → `review` → `done`. Any active spec can become `blocked` with a documented reason. Update `spec.md` and `state.md` statuses together.

## `spec.md` contract

YAML frontmatter: `id`, `title`, `source` (`github|jira|manual`), `source_ref`, `created` (YYYY-MM-DD), `status`, `priority` (`P1|P2|P3`), `depends_on` (list; use `[]` if empty), `branch` (ticket ID).

Required sections, in order:
1. **Context**
2. **Research Findings** — Relevant Files, Existing Patterns, Constraints
3. **Requirements** — Must Have, Should Have, Won't Do
4. **Tasks** — ordered T1..TN checklist; every task specifies Files, Approach and Done when
5. **Verification** — Automated Checks, Manual Checks, Acceptance Criteria

## `state.md` contract

YAML frontmatter: `id`, `status`, `branch` (or `null` until execution), `current_task` (T<N> or null), `started` (date or null), `updated` (date).

Required sections:
1. **Progress**
2. **Task Log** (append-only)
3. **Decisions**
4. **Blockers**
5. **Verification Report(s)** (append-only reports)

## Operating rules

- Treat specifications as contracts. If an implementation plan must change, revise and review the spec before diverging.
- `state.md` tracks actual progress. Never erase earlier Task Log entries, verification reports or decisions.
- One feature branch per ticket (`<ticket-id>`) and one focused implementation commit per task: `<ticket-id>: T<N> - <description>`.
- Tracking commits and verification commits may be separate from implementation commits.
- Never discard unrelated work or make network/push changes without user consent.
- Do not mark checks PASS unless actually executed. Be explicit about skipped or unavailable checks.
- Do not silently replace other `AGENTS.md` instructions; root and nested directory instructions also apply.

## Skill contracts

| Artifact | Writer | Reader |
|---|---|---|
| `spec.md` | `$spec-plan`, tracking changes by `$spec-execute`/`$spec-verify` | `$spec-execute`, `$spec-verify`, `$spec-list` |
| `state.md` | `$spec-plan`, `$spec-execute`, `$spec-verify` | `$spec-execute`, `$spec-verify`, `$spec-list` |
| `.specs/AGENTS.md`, root/nested `AGENTS.md` | `$spec-steer` after approval | all relevant skills |
| No `.specs/` outputs | `$spec-quick` | none |

Invoke skills by `$spec-plan`, `$spec-execute`, `$spec-verify`, `$spec-steer`, `$spec-quick`, `$spec-list` (Codex syntax; not Claude's slash commands).
