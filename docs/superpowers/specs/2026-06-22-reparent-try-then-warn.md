# Reparent: try-then-warn (never get stuck)

> Plain-language spec agreed with the user on 2026-06-22.

## The philosophy (one line)

**Crowbar auto-does the easy moves, never gets stuck, and clearly warns you on the hard ones. It never rebases silently — that stays the user's call.**

## What a "reparent" is

Moving a branch (a small stack of edits) to sit on top of a different base. Today
this is done by `git rebase --onto` (replaying the branch's commits onto the new
parent). If the branch's edits and the new parent both touched the same lines,
the rebase conflicts.

## The bug we're also fixing

Today, when a reparent rebase conflicts, the worktree is left **mid-rebase**
(detached HEAD + conflict markers) onto the new parent, but the read model still
says the **old** parent and only a `LastError` is set — a stuck, divergent state.
That is the "weird state" the user saw. We replace it entirely.

## New behavior

### On reparent (drag a branch onto a new parent)

1. **Try the rebase** onto the new parent.
2. **Clean** → the branch moves under the new parent and is fully integrated.
   Done, no warning.
3. **Conflict** → **abort the rebase** so the worktree returns to a clean state
   (never the stuck halfway mess), **but keep the branch moved** under the new
   parent (update its parent; do NOT snap it back to the old one). The branch is
   now "moved on paper, but not yet integrated."

The conflict check is always **live/fresh** — a parent can move on over time, so
cleanliness is recomputed at the moment of the action, never remembered.

### Surfacing a moved-but-conflicting branch

- **Tree row:** an amber ⚠️ overlay on the row — at a glance, "this one
  conflicts with its parent." Driven by the existing `mergeConflicts` prediction
  (already on the DTO, recomputed per-read), which the tree currently ignores.
- **Branch panel:** a clear message: *"⚠️ Conflicts with `<parent>` — this branch
  was moved here but its changes clash with `<parent>`, so it isn't merged in
  yet."* plus a user-initiated **"Rebase onto `<parent>`"** button and a hint that
  moving it back undoes it.

### The "fix" (user-initiated only)

- The **automatic** attempt (on drop) backs off cleanly on conflict — Crowbar
  never forces it.
- The **"Rebase onto `<parent>`" button** is the user choosing to do it. When
  clicked, Crowbar runs the rebase and **keeps** it on conflict so the user can
  resolve the markers and finish (the same resolve flow the merge path uses).
- Rule preserved: **Crowbar never rebases silently; it only gives a one-click way
  to do it when the user decides.**

### Moving back (drag back to the original parent)

Same try-then-settle: Crowbar tries the rebase **right then** (fresh — the
original parent may have drifted). Clean → it lands back and tells the user
"all clean under `<original parent>`"; conflict → it stays moved-back with the
warning.

## Honesty notes

- The warning is a heads-up, not a guarantee: if the live check can't run, the
  row shows nothing rather than a false alarm.
- On a conflicted (aborted) reparent the commits are **not** replayed onto the
  new parent yet — "moved on paper" — which is exactly what the ⚠️ and the panel
  message communicate.

## Key implementation decisions

- **No more stuck rebase:** the reparent path must `git rebase --abort` on
  conflict (it never does today) and persist the new parent regardless of the
  conflict.
- **Parent persist without a clean fork point:** the existing Reparent command
  requires both ParentID and ForkPointSha; a conflicted move has no integrated
  tip. Persist ParentID with the **merge-base of (child, new parent)** as the
  fork point so diffs/summaries read correctly against the new parent's lineage;
  a later clean rebase (the button) finalizes the fork point to the new tip.
- **Conflicted-but-moved needs no new status:** it's `parentId = new parent` +
  a clean worktree + `mergeConflicts = true` (computed live). The warning and the
  panel state both fall out of `mergeConflicts`.
- **Cleanup:** the `spike/thingy` workspace currently sits in the old stuck
  mid-rebase state and must be returned to clean as part of this work.

## Out of scope / not doing

- jj-style first-class conflicts (a branch genuinely "moved AND conflicted" at
  once) — impossible with real git refs; the overlay model above is the best
  faithful approximation.
- Auto-resolving conflicts for the user.
