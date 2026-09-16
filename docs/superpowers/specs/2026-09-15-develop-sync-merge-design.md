# Sync `restyling/v2` with `develop` — merge design

**Date:** 2026-09-15

**Status:** DRAFT — proposed, not yet implemented.

**Scope:** merging `origin/develop` (tip `493991137`) into `restyling/v2`,
preserving `restyling/v2`'s UI restyle and its chat-usecase backend refactor,
while bringing in every fix and feature `develop` gained in the meantime —
most notably native OS context menus. Covers the merge strategy, the
per-cluster conflict-resolution policy, and the verification plan.

**Not in scope:** re-litigating any design decision already made on either
branch; this is a sync, not a redesign. No new features beyond what `develop`
already has. No rebase — history on both branches is preserved as-is.

---

## 1. Actual divergence

`restyling/v2` already contains one historical merge of `develop`
(`c2735dc66`), so the branches are much closer than "451 commits vs 0"
suggests:

- `git merge-base restyling/v2 origin/develop` = `0043620fc`
- `origin/develop` has **11 commits** past that base restyling/v2 lacks
  (ending at `493991137`)
- `restyling/v2` has 451 commits past that base develop lacks — this is the
  restyle plus the chat-usecase backend refactor plus everything else built
  since the last sync

The 11 missing commits, oldest to newest:

| Commit | PR | What it does |
|---|---|---|
| `a93ce749e` | #164 | Chat attachments: reorderable cards, Excalidraw takeover |
| `c652278ee` | #166 | **Native context menus**, replacing every React-rendered right-click menu |
| `d6b5e2c75` | #168 | Dependabot: 4 Tauri plugin cargo bumps |
| `9b98c585b` | #169 | Codex protocol streaming completion, pane/connection lifecycle fixes |
| `1362209ce` | #170 | Codex working-state desync, Stop not halting, compaction/attachment UI |
| `25dda19c1` | #171 | Codex working-status, streaming render, scroll, terminal approval, stale-thread |
| `a265f880f` | #174 | Storage-integrity: home propagation, hook-spool wedging, orphaned blobs |
| `d9c2b3d5d` | #173 | Sub-agent bleed-through, scroll anchor, composer corruption, pane desync, prompt durability |
| `7fc2d8bc1` | #175 | Hook-spool drain loop leaking a connection per delivery |
| `7cf9aaebc` | #176 | Subagent leak, reopened codex memory-session chat theft |
| `493991137` | #177 | Provider switching: atomic switch+send, handoff-context fix, subagent tracking |

None of these are cosmetic — every one is either the requested native-menu
feature or a real bug fix. Nothing here gets dropped; the question is only
how each survives contact with `restyling/v2`'s refactored/restyled code.

## 2. Overlap surface

Of develop's 436 changed files and restyling/v2's 1138 changed files (both
since the shared base), **90 files were touched by both sides**. That's the
entire conflict surface — everything outside this list applies from either
side with no collision. It splits into four clusters with different risk
profiles and different resolution policies.

### 2.1 Backend chat-runner internals (~50 files, highest risk)

`api/internal/app/usecases/chat/internal/runner/*` and its siblings
(`activity.go`, `event_store.go`, `hub.go`, `chat.go`, `turn.go`, `container.go`,
route/handler files). This is where **the backend refactor** lives —
`52562102f` "decompose the chat usecase, and enforce the shape with tests" —
restyling/v2's own structural work in this exact directory, done after the
merge base.

Risk: develop's 7 bug-fix PRs (#169–177) patch behavior in the *pre-refactor*
shape. Git's 3-way merge works on text, not semantics — a clean merge (no
conflict markers) here is not proof the fix's behavior survived; the patch
context could apply to the wrong spot in a restructured file and still parse.
**Policy:** run the merge, then for every file in this cluster, cross-check
against the originating develop commit's diff and confirm the specific fix
(subagent bleed-through, hook-spool leak, storage-integrity, atomic
switch+send, Codex working-state desync, etc.) is present and reachable in
the refactored structure — not just that the file merged cleanly. Add a
regression test per fix that doesn't already have one (per this project's
own black-box-regression-test convention), so the fix is provable, not just
eyeballed.

### 2.2 Web UI: styling + shared interactive surfaces (~25 files)

`styles/activity.css`, `styles/composer.css`, `styles/transcript.css`,
`agent-chat-view.tsx`, `agent-chat-pane.tsx`, `pane-container.tsx`,
`workspace-view.tsx`, `terminal.tsx`/`terminal-tab.tsx`, `use-transcript-anchor.ts`,
`agent-chats-slice.ts`.

**Policy:** restyling/v2's visual treatment (colors, spacing, tokens,
animation) is the one we keep — the user was explicit the restyle must stay
exactly as it is. But every functional delta from develop (transcript
scroll-position restore, terminal attach/reconcile fixes, new activity
states, subagent-tracking UI) must survive and be skinned to match the
existing restyle, not dropped for being inconvenient to merge. This is a
per-file manual reconciliation, not a bulk "ours" or "theirs".

### 2.3 Native context menus + bridge (2 files, low risk, verify anyway)

The actual menu components (`web/src/components/ui/context-menu.tsx`,
`block-context-menu.tsx`) were **never touched by restyling/v2** — they
should apply from develop with no conflict, bringing native OS menus in
wholesale. `web/src/lib/crowbar-bridge.ts` and `desktop/src-tauri/src/lib.rs`
*were* touched by both sides and need manual reconciliation (both add
distinct bridge commands/Tauri setup — neither side's additions should be
lost).

**Policy:** take develop's context-menu component files as-is. Manually
merge the bridge/lib.rs additions. Verify every surface that used to have a
React right-click menu now shows the OS-native one (this can't be checked by
`crowbar-driver`/parity tooling — native menus aren't in the element tree —
so it's a manual `make dev-desktop` check per surface: sidebar rows, chat
messages, editor, terminal).

### 2.4 Mechanical (lockfiles, deps, tests)

`Cargo.lock`, `bun.lock`, `package.json`, and the ~13 test files in the
overlap set. **Policy:** resolve source first, then regenerate lockfiles
(`bun install`, `cargo generate-lockfile` / `cargo check`) rather than
hand-merging them. Test files follow whatever their production code ends up
looking like — re-run/rewrite assertions against final behavior, don't
merge test bodies blindly.

## 3. Merge strategy

`git merge origin/develop` on `restyling/v2`, not rebase. The branch is
pushed (`origin/restyling/v2`) and rebasing 451 commits for a sync that a
plain merge handles just as well is unnecessary history churn and risk for
no benefit.

Execution order, each step committed and verified before the next:

1. Non-conflicting develop-only files (346 of 436) come in automatically —
   confirm they build once the merge lands.
2. Cluster 2.3 (bridge + native menus) — smallest, well-understood, unblocks
   an easy live-verification win early.
3. Cluster 2.1 (backend runner) — largest and highest-risk; do this with the
   most scrutiny, one develop PR's worth of fixes at a time.
4. Cluster 2.2 (web UI/styling) — depends on nothing above; can happen in
   parallel with (3).
5. Cluster 2.4 (lockfiles/mechanical) — last, after source is settled.

Given the independence of clusters 2.1 and 2.2, they can be worked
concurrently once the merge's conflict set is materialized.

## 4. Verification

- `tsc` (web) and `cargo build`/`cargo check` (desktop) after every cluster.
- Targeted/modified-file tests only for files touched by the merge — never a
  full-suite run.
- Live check in `make dev-desktop` (never headless) covering: chat
  streaming, transcript scroll-position restore, provider switch
  (atomic switch+send), terminal attach/reconcile, subagent lifecycle, and
  right-click menus on every surface that used to render one in React —
  confirming both that native menus now appear and that no restyled visual
  regressed.
- No PR/push beyond what's asked — this stays as commits on `restyling/v2`
  unless told otherwise.
