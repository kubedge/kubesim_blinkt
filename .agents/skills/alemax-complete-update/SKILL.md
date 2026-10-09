---
name: alemax-complete-update
description: "Use when `.local/HANDOFF.md` names a delivery for this project. Complete a meta delivery from the receiving project’s Codex session: locate the branch, preserve customizations, verify Codex skills, run checks, and follow the project’s PR process."
license: MIT
metadata:
  author: alemax
  version: "2.0"
  source_reviewed_model: claude-5
  source_reviewed: 2026-09-04
---
<!-- The Codex CONVERSION is cloudison/photo-common's, taken verbatim: the
     $-invocations and the host-difference notes are
     hand-written work, not a transform of the Claude body.

     Its CONTENT is from alemax 2.0 (source_reviewed 2026-09-04), so where the Claude
     body at templates/claude/skills/complete-update/SKILL.md has moved on, this has not.
     Reconcile the prose; do not re-derive the conversion. -->

## Working directory

Keep the working directory at the repository being operated on; every command below
runs from its root. `alemax` is the packaged CLI under `plugins/alemax/`.
User instructions and existing authorization take precedence over workflow defaults;
ask only for missing decisions or actions outside the authorized scope.

## Workflow

Run in the receiving project, never from the meta-repo. The helper locates a
broadcast or sync delivery; it does not apply, commit, or push it.

```bash
uv run --directory plugins/alemax alemax update context
uv run --directory plugins/alemax alemax update handoff
uv run --directory plugins/alemax alemax update locate --json
```

1. Read the handoff and respect the context check. Multiple refs require selection;
   no handoff or ref means inspect remaining gaps before reporting current.
2. Preserve untracked bootstrap files listed by `locate` before applying a delivery
   that would overwrite them. Review and commit only those intended files; never
   delete them or force the application. Follow the project's branch/PR policy.
3. Use the reported verb: cherry-pick a broadcast tip, merge a sync. A broadcast's
   parent is its three-way base; merging that unrelated branch would lose local
   customizations. Resolve conflicts using project context and preserve edits.
4. Follow the actual payload: re-lock changed manifests and inspect `citrim` for
   foreign-stack CI jobs. Claude settings reconciliation, if requested, still uses
   the upstream Claude floor; it does not configure Codex permissions. Verify
   `AGENTS.md`, `.agents/skills`, and the Codex plugin separately.
5. Run `stale` to inspect Codex discovery entries. Links to the delivered plugin are
   the intended repository surface, not obsolete duplicates. Divergent real skill
   directories need review before replacement. This helper removes nothing and
   does not mark Claude skills or command stubs for deletion.
6. Run the project's checks, commit on its delivery branch, push and open a PR.
   Merge only within existing authorization after required checks pass. Clear
   completed handoff items after verifying the result. Preserve meta-owned sibling
   worktrees until the sender handles cleanup. Report missing Codex artifacts as
   a propagation-policy gap; do not fetch unrelated meta files by hand.
