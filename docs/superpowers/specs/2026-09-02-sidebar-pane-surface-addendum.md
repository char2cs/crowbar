# Sidebar and pane surface — addendum

**Date:** 2026-09-02

**Status:** amends [`2026-08-28-sidebar-and-pane-surface-design.md`](./2026-08-28-sidebar-and-pane-surface-design.md)
(the closed frontend spec, "the nine laws"). Nothing here is a new design —
it is the fourth attempt to state the same law precisely enough that a build
stops drifting off it. Three earlier builds on this branch each violated it a
different way; this document exists so the next one has no ambiguity left to
violate.

**Before reading this:** most of what prompted it was already correct in the
2026-08-28 spec and simply not built — that half needs an audit, not a
decision. This document covers only the parts that were genuinely open or
genuinely wrong.

---

## 1. Row actions: Fork and Thread are two buttons, not one contextual mark

**Revises §3.1.** The closed spec gave a row one contextual `+`: workspace on
a git-capable parent, thread (elbow mark) otherwise, never both. That is
superseded. A row now carries both, always, as two separate buttons — which
one applies depends on nothing; both are always legal because both always mean
the same thing (fork = new workspace off this branch, thread = new chat
sharing this workspace).

Trailing cluster, revised:

```
[ glyph ] [ label ]   [ Fork ] [ Thread ] [ ⌄ ]
```

- **Fork** — mints a new workspace (new worktree, new branch) whose parent is
  this row's chat. 1:1 workspace↔parent-chat, per the hierarchy invariant.
- **Thread** — mints a new chat that runs in the **same** workspace as this
  row's chat. No new worktree.
- **⌄ (dropdown)** — takes the trailing slot the fold chevron and the trash
  both used to occupy. See open question in §4 below for what it holds beyond
  fold/expand.

**Trash is no longer a row button.** Deletion moves to the drag gesture in §2.
A row that owns nothing to fold (a leaf chat with no forks) still shows the
dropdown; it just has no fold action to give it.

---

## 2. Deleting: drag a row onto the file explorer card

**Revises §9, and resolves the rejection recorded in §10** ("Drag-to-trash on
the card — puts an irreversible drop in a gesture space that is otherwise
entirely additive... and it only exists while dragging, so it teaches nothing
when you go looking"). Both objections were real when they were written. Both
are answered by wiring this into machinery that already exists and that the
codebase already learned this exact lesson on once:

- `lib/store/sidebar-removal.ts` — the removal tray. A held row drains 8s with
  an undo (`Keep`), or for a cascading kind (repo, project) asks once via
  `removal-confirm-dialog.tsx` before anything commits. **This already answers
  "irreversible."** Its own doc comment records the prior version of this
  exact gesture — a raw drop-to-delete zone with nothing to undo — being
  replaced by this tray for that reason (`removal-tray.tsx`: "This replaces
  the drop-to-delete zone that used to live here. That zone deleted on
  release with nothing to undo; a tray is what makes the gesture safe enough
  to be the only removal path in the sidebar.")
- `components/tree-dnd/drop-dom.ts`'s `DropZone<S, Hit>` — a generic
  whole-region drop target that is not a row, already built for exactly this
  shape of thing ("the editor pane, standing for removal" — the same prior
  gesture, deleted with the old Chats panel in Task 22). Reuse it; do not
  build a second whole-region hit-test.

What's actually new is only the choreography, and it is specific:

1. A row drag starts (existing 5px threshold, §8).
2. **If the file explorer card is open, it folds** — its existing §6.4 fold
   treatment — and this fold fires **only** because a drag is live; it is not
   a user click and must not be recorded as one (no state change to whether
   the user last had it open).
3. Once folded (or immediately, if it was already folded), the card's surface
   becomes a trash-can drop target — visually matching what `develop` already
   ships for this today. Verify against a live `develop` build when
   implementing; do not redesign the visual from scratch.
4. Dropping on it feeds the **same removal tray** every trash-button delete
   already uses — `commitRemoval`/`useRemovalTrayStore`, unchanged. The held
   row (draining, or waiting on a cascade confirm) renders **at the top of the
   file explorer card**, not at the sidebar's separate foot position it
   currently occupies (`sidebar-tree-chrome.tsx` mounts `<RemovalTray />`
   sidebar-wide today; it needs to move into the card).
5. When the drag ends — drop or release outside the target — **the card
   returns to exactly the open/closed state it was in before step 2.** A fold
   triggered by a drag must not persist past the drag.

Same refusals as everywhere else in §8.3 apply here without change: a working
chat may not be dragged, so it may not be dragged to trash either.

---

## 3. Locked containers are folders, not workspaces

**Clarifies, does not revise, §9's protected-branch carve-out** ("A protected
branch is the repo's own ground — `develop` and `main` are not workspaces you
made, so they are not workspaces you can delete"). Generalize the same fact to
click, not just to trash:

**A project, a repo, and a locked/protected branch never open a pane and never
open a workspace of their own on click.** Clicking one only ever expands or
collapses its children in the tree — the same chevron behaviour a folder row
already has. They contain rows; they are not rows you check out into. `main`
being clickable into a live pane (as seen in this session) is the bug this
closes: it was being treated as an ordinary workspace row instead of as a
container.

This does not touch forking: forking off a locked branch (Fork on its row, per
§1) still creates a real, deletable workspace — the lock protects the branch
itself from being treated as disposable, not from being built on.

---

## 4. Decided: no dropdown Delete item

**Closed.** Drag-to-trash (§2) is the only way to delete. The dropdown (⌄)
never carries a "Delete" item — this matches production Crowbar today and
stays that way. The discoverability half of the old §10 objection is
accepted as a known, deliberate cost, not a gap to patch; only the
irreversibility half needed fixing, and §2's removal tray fixes it.

---

## 5. The affordance row (spec §3.5) is now redundant for almost every row it fires on

**Revises §3.5, as a direct consequence of §1.** §3.5 was written for the OLD
row anatomy — a single contextual `+` that could only make ONE of
{workspace, thread} depending on the row. Under that model, a container
genuinely needed a separate affordance row for the case its own `+` couldn't
serve (a workspace row's `+` made a fork, not a thread — so the first chat in
it had nowhere else to come from).

That reason is gone. §1 gives every workspace/branch/chat row its own,
always-present Fork **and** Thread buttons. A chat row with no thread yet is
not missing an affordance — its own Thread button already is one. Rendering
a second, nested, unlabeled affordance row underneath it is now pure
duplication, and it reads as broken: a bare icon with no label, no branch
name, nothing — indistinguishable at a glance from a corrupt or empty entity.
This is confirmed live: it is what produces the mystery blank rows seen under
every childless chat in the running app.

**The fix:** the affordance row renders **only for a container that does not
itself carry Fork/Thread** — concretely, a **project** or **repo** row with no
branches under it yet. Those rows have no owning chat to fork or thread from
(Fork/Thread both require a parent chat, per §1), so they are the one case
that still needs a way to bootstrap a first branch/workspace from nothing. A
**workspace/branch row or a chat row must never render a nested affordance
row for its own missing children** — its own trailing Fork/Thread already is
the only way in, exactly as a row's own buttons are everywhere else in this
design.

## 6. Locked/protected branch rows must carry a distinct glyph

Not previously specified, and confirmed live as wrong: `main` currently draws
the same `GitBranch` mark as an ordinary workspace row. `workspace-branch-icon.tsx`
already has a correct `Lock` glyph for `status === 'locked'` — the bug is that
whatever builds the repo-home/protected-branch row either doesn't pass
`status: 'locked'` through to it, or doesn't route that row through
`WorkspaceBranchIcon` at all. Fix it so a locked/protected branch draws the
lock glyph, matching the icon component's own existing (unused) case.

---

## 7. Standing instruction: fidelity against `develop`

The user's own framing, verbatim, matters more than any single item above:
*"the sidebar integration is trash compared to what develop has for
workspaces and chats."* Production Crowbar (`develop`) is the bar, not just
this spec's text read literally. Where the two disagree on a point this
document doesn't explicitly settle, a live side-by-side against `develop`
outranks a guess from the spec's prose — check the running app, don't infer
from static reading alone.

---

## 8. Next step

This document plus the closed spec is now the complete law. Before any code
changes: audit the current build against both — every law in §1 of the
2026-08-28 spec, plus §§1–4 and §§5–6 here — and come back with a punch list
of what's actually wrong, file by file, rather than fixing on sight. Given
this is the fourth drift, the audit is the part worth doing carefully.
