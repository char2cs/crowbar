# Views are records, not inferences

Recents rows disappear on their own. Not often, and never for a reason the user
caused — a workspace unmounts, a keep-alive pass reclaims a store, a navigation
tears something down, and a row the user opened is gone. This spec is about
making that impossible rather than fixing it again.

## What is actually wrong

A view's **existence is inferred from its panes**. `deriveRecentsEntries`
(`components/sidebar/lib/recents-entries.ts`) builds the band by walking
`panes`, grouping them with `viewIdOf(pane)`, and treating the result as the set
of open views. A view exists because a pane references its id. So any code that
clears a pane's `chatId` deletes a view as a side effect, whether or not it
meant to.

`dormantArrangements` is the patch for that, and it is the evidence the shape is
wrong. It exists only to remember a chat whose pane is about to go, and it is
maintained by hand at **nine write sites inside `pane-slice.ts`** plus two more
in `recents-actions.ts`. Eleven places that must each remember to preserve an
invariant none of them owns. The live bug — creating a branch while a
project-home chat is open silently drops that chat's row — is one site that
didn't.

This is not a bug with a fix. It is a generator: every future teardown path is
one more chance to miss it.

## The insight this spec turns on

**The view is already a durable, first-class, project-scoped thing.** Four
parallel maps are keyed by `viewId` and every one of them is persisted through
`saveWorkspaceLayout` (`features/panes/stores/window-pane-store.ts`):

| map | what it holds |
| --- | --- |
| `parkedViews: Record<viewId, LayoutNode>` | the view's own layout tree while off screen |
| `viewProjects: Record<viewId, projectId>` | which project the view belongs to |
| `activeViewByProject: Record<projectId, viewId>` | where each project was last looking |
| `recentsOrder: string[]` | the band's order |

`parkedViews` is the tabs model already: an off-screen view keeps its entire
layout. What is missing is one field — a record saying the view *is open* —
independent of whether any pane currently points at it.

So this is not a new subsystem. It is a **consolidation**: four maps keyed by
the same id, plus the existence fact they all presuppose, become one record.

## Target model

```ts
interface ViewRecord {
  id: string
  projectId: string
  chatIds: string[]      // membership — see "the fork", below
  layout: LayoutNode     // was parkedViews[id]
  openedAt: number
}

views: ViewRecord[]      // ORDER IS THE BAND'S ORDER
activeViewId: string
activeViewByProject: Record<string, string>
```

`views` replaces `parkedViews`, `viewProjects`, `recentsOrder` and
`dormantArrangements` outright. `activeViewByProject` survives because it
answers a different question (where a project was last looking), not an
existence one.

### What derives, and only at read time

`RecentsEntryState` (`live` / `working` / `set` / `dormant`) and `showing` stay
derived exactly as they are today, by `deriveRecentsEntries` — but from
`views` as the authority for *which entries exist*, with panes, the working map
and `activeViewId` supplying only decoration:

- `live` — some pane holds one of the entry's chats
- `working` — the working map says so
- `set` — `chatIds.length > 1`
- `dormant` — none of the above; **no record required**, it is the absence of a pane
- `showing` — `id === activeViewId`

`dormantArrangements` disappears because "dormant" stops being a thing to
remember and becomes a thing to notice.

## Invariants

These are the spec. Everything else is implementation.

1. **Only a user gesture changes `views`.** Exactly four writers: open a view,
   close a view (the row's ×), reorder (drag), remove a member (the per-chat ×).
   Nothing else may write it — not teardown, not eviction, not navigation, not
   keep-alive, not a runner moving.
2. **Panes are a projection.** Any code may null a pane's `chatId` at any time
   for any reason. It has no effect on `views`.
3. **Reconciliation is one-way.** Panes are brought into line with `views`,
   never the reverse. The showing view has panes; panes whose chat no view
   claims are dropped.
4. **A view's project is fixed at open.** It is how the band scopes rows without
   consulting a workspace store that may not be mounted — the thing
   `recents-for-project.ts` currently rebuilds from `getHomeTree` plus every
   active store, and the reason a project-home chat behaves differently from a
   repo chat today.

## The fork, and my recommendation

**Who owns membership — which chats are grouped into one view?**

Today the layout owns it: `drop-actions.ts` records that grouping used to need a
second write (`groupIntoArrangement`) which drifted from the layout, and
deriving it from panes fixed that drift. That fix was correct for the bug it
addressed.

**Recommendation: membership moves onto the record (`chatIds`), and the layout
reconciles to it.** The drift that motivated deriving it cannot recur under
invariant 3, because there is no second writer to drift *from* — the layout is
downstream by construction. Keeping membership on the layout would leave
existence and membership under two different authorities, which is the same
class of bug at a narrower seam.

Consequence to accept deliberately: a merge (`mergePaneIntoView`) and a split
become writes to `chatIds` that the layout then follows, rather than layout
moves the band reads off. That is a real change to the drag-and-drop path and
is where I expect the implementation to need the most care.

## Staging

Each stage is independently green, independently revertible, and independently
live-verifiable.

**Stage 1 — introduce the record, behaviour-neutral.**
Add `views`, written by the same four gestures that exist today. Make
`deriveRecentsEntries` take *existence* from `views` while still reading panes
for state. Leave `dormantArrangements` in place and still written. Nothing
observable changes; the two structures agree.

**Stage 2 — delete `dormantArrangements`.**
Remove all nine sites in `pane-slice.ts` and the two in `recents-actions.ts`.
Archive-on-evict becomes a no-op: the record already exists. This is the stage
that fixes the live bug.

**Stage 3 — panes become a projection.**
Add the reconciler. Teardown, keep-alive eviction and workspace unmount stop
being special-cased around Recents entirely.

**Stage 4 — absorb `parkedViews`, `viewProjects`, `recentsOrder`.**
Fold them into `ViewRecord` and update `saveWorkspaceLayout`'s payload. Pure
consolidation, no behaviour change.

Membership (the fork) lands with Stage 4, not earlier — it touches the
drag-and-drop path and should not be entangled with the bug fix.

## Proof obligations

A stage is not done because its tests pass. It is done when these hold.

1. **The property test that makes this real** — writable only once `views` is
   authoritative, which is the point:

   > For any sequence of pane mutations — teardown, eviction, navigation,
   > split, merge, workspace unmount, runner move — containing no user close
   > gesture, the set of Recents entries is unchanged.

2. **Every existing recents test passes untouched.** They encode the visible
   behaviour; changing one is a signal the refactor moved something the user
   can see. If one must change, it is named and justified, never quietly
   rewritten.

3. **The live bug, as a regression test**: project-home chat open in Recents,
   create a branch, assert the row survives — plus its repo-chat twin, since
   today's asymmetry between them is the tell.

4. **Live verification per stage** on `make dev-desktop`, not headless.

## Persistence

`saveWorkspaceLayout` is debounced (300ms) and already carries
`parkedViews`, `viewProjects`, `activeViewByProject`, `activeViewId`,
`recentsOrder` and `dormantArrangements`. Stage 1 adds `views` to that payload;
Stage 4 removes the four it replaces.

Pre-production, so **graceful fallback, no migration**: a persisted payload with
no `views` key rebuilds the records from `panes` + `parkedViews` + `recentsOrder`
exactly as today's derivation does, once, at load. A payload that has `views`
ignores the legacy keys.

## What this does not change

The band's appearance, its gestures, `viewIdOf`, pane splitting, the editor
buffers, or anything in `api/`. This is entirely `web/src/features/panes` and
`web/src/components/sidebar`.

## Open questions

- Does any consumer outside the band read `dormantArrangements`? Stage 2 must
  survey before deleting, not assume.
- `activeViewByProject` and `views[].projectId` overlap. Keep both (they answer
  different questions) or derive one? Decide at Stage 4, not before.
