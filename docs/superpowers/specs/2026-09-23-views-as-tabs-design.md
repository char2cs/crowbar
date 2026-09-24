# Views as tabs

Supersedes `docs/plans/2026-09-23-views-are-records.md`.

## Problem

A Recents row disappears without the user closing it. The live repro: a
project-home chat is open, the user creates a branch, and the new branch chat
lands in the home chat's pane; the home chat's row is gone.

Two structural faults make this class of bug inevitable rather than
occasional:

1. **A row's existence is a disjunction of three sources.**
   `deriveRecentsEntries` draws a row if a pane holds the chat, OR the
   workspace's `working` map says it is running, OR `dormantArrangements`
   remembers it. Rows move between those sources by hand-written handoffs at
   eleven sites (nine in `pane-slice.ts`, two in `recents-actions.ts`). A
   missed or guarded-off handoff deletes a row. `setPaneChat` skips its
   archive when the evicted chat is working, and `recentsForProject` only
   reads `working` from mounted workspace stores, so a working chat whose
   store is not mounted falls through all three.
2. **`setPaneChat` is three operations behind one name.** It updates a
   runner on the same chat, swaps a different chat into an occupied pane,
   and empties a pane when Law 4 evicts it. The swap is the only reason an
   archive exists at all, and it is reachable from ordinary clicks
   (`openAgentChat`).

## Principle

**A Recents row is a view record. Nothing else.** The row exists exactly
as long as the record does, and the record changes only through a small,
named set of actions. Everything else the band shows about a row (working,
showing, grouped) is a label computed at render time and can never create
or remove it.

## Model

```ts
interface ViewRecord {
  id: string
  projectId: string   // fixed when the record is created (Law 1)
  layout: LayoutNode  // this view's tiling tree; its leaves are its panes
}

interface PaneSlice {
  views: Record<string, ViewRecord>   // by id, O(1) lookup
  viewOrder: string[]                 // window-wide band order
  activeViewId: string | null         // null = the stage is on screen
  activeViewByProject: Record<string, string>
  stage: LayoutNode                   // the chatless New Tab surface (Law 6)
  panes: Record<string, PaneGroup>    // flat, unchanged shape…
  bottomLayout: LayoutNode            // unchanged; window tray, not a view
  // …activePaneId, mostRecentActivePaneIds, fullscreenPaneId, activeProjectId
}

interface PaneGroup {
  // …existing fields
  viewId: string | null   // REQUIRED; null only for stage and bottom-tray panes
}
```

The band for a project is `viewOrder.filter(id => views[id].projectId === p)`.

**A group is a view whose layout has more than one pane.** Its chats are its
panes' `chatId`s, read in layout order. There is no second record of
membership, so a group cannot disagree with what is on screen.

**The stage is not a view.** It is the one chatless surface a project with
nothing open shows, and it is never a row. It may hold chatless panes (editor
tabs, terminals). When a chat lands in a stage pane, the stage is promoted:
its layout becomes a new record's layout, its panes take the new `viewId`,
and a fresh empty stage replaces it. This is the same transition as today's
"tag the untagged empty view", made explicit.

### Removed

| removed | replaced by |
| --- | --- |
| `dormantArrangements` and its 11 write sites | nothing: there is no dormant state |
| `parkedViews`, `rootLayout` | `views[id].layout`; the renderer reads the active record |
| `viewProjects` | `views[id].projectId` |
| `recentsOrder` | `viewOrder` |
| `viewIdOf`'s `?? pane.id` fallback | required `viewId` |
| `partitionLayoutByView`, `restoreViewProjects` and the other old-shape readers in `hydrate.ts` | nothing (pre-production, no migration) |
| `forgetDormantArrangement`, `removeChatFromDormantArrangement` | nothing |
| the swap half of `setPaneChat` | nothing: swapping is not an operation |
| `RecentsEntryState` `'live'` / `'dormant'` | every row is an open view |

## Invariants

These are the spec. They are checked by `assertViewIntegrity(state)`, run
after every pane action in dev and in every test.

1. **Every pane belongs to exactly one place**: a record's layout (with that
   `viewId`), the stage (`viewId: null`), or `bottomLayout` (`viewId: null`). Every layout
   leaf names an existing pane.
2. **Every record holds at least one chat pane.** A record left with none is
   removed in the same action that emptied it. Only four things can empty
   a view: a close, a chat deletion, a merge moving its last chat out, and a
   Law 4 eviction. All four are in the table below.
3. **One pane per chat** (Law 4, unchanged).
4. **`viewOrder` and `views` hold the same ids.**
5. **A pane's `chatId` never changes from one chat to a different chat,**
   except through `retargetPane` (below). Filling a chatless pane is not a
   change of chat.

## Actions

Structural writes go through three internal helpers in a new
`features/panes/lib/view-ops.ts`: `insertPane`, `removePane` and `movePane`.
They are pure functions over the slice's draft, and each keeps layout
leaves, `panes`, `views`, `viewOrder` and the active pointers consistent,
including removing a record left without a chat (invariant 2) and picking the
next view to show. No action edits those fields directly. That is what makes
the eleven-site class of bug unwritable: there is one place to get it right.

Public actions, and the only ways a row comes or goes:

| action | trigger | effect on rows |
| --- | --- | --- |
| `openChat(chatId)` | sidebar click, ⌘N, New Tab list, new branch/fork/thread | reveals the view holding it; otherwise a **new record** appended to `viewOrder`, or the stage is promoted if it is showing |
| `dropChatOnPane(chatId, paneId, zone)` | drag onto a pane | fills a chatless pane in place, or splits the target; either way the chat **joins the target's view**; an already-open chat is moved (`movePane`) and its old view is removed if it held nothing else |
| `detachPane(paneId)` | drag a member out of a group | **new record** placed directly after the group |
| `closePane(paneId)` / `closeView(viewId)` | the × (per chat / per row) | removes the pane(s); a view left without a chat is **removed**; `releaseClosedChat` runs as today. Close stays atomic and final, with no leftover row |
| `reorderView(id, targetId, mode)` | band drag | moves the id within `viewOrder` |
| `forgetChat(chatId)` | the daemon's `deleted` frame | removes its pane; the view is **removed** if that was its last chat |
| `retargetPane(paneId, chatId, runnerId)` | the runner stream's `moved` frame only | the pane's view keeps its row and now shows the chat the runner entered; the chat it left keeps its tree entry but has no row. Any other pane showing the entered chat is removed (Law 4), and its view goes if that was its last chat |
| `setPaneRunner(paneId, runnerId)` | runner placement on the same chat | none |
| `adoptBackgroundChat(chatId, projectId)` | a chat turns working with no record | **new record**, appended, not activated |

Everything that is **not** in this table cannot add or remove a row:
teardown, keep-alive eviction, workspace unmount, navigation, project
switches, reconnect seeds, and hydrate.

### Decisions this table encodes

- **Opening a chat never replaces what a pane shows.** This deletes the path
  the live bug came through. `openAgentChat` and `openChatIdInOwnView`
  collapse into `openChat`.
- **No dormant rows.** Closing was already made final; the only remaining
  producers of dormant rows were the swap-away archive (deleted) and a
  `/clear` (see `retargetPane`: the row carries on with the new
  conversation, and the old one is still in the tree). A row is always a
  view you can switch to.
- **A chat that runs with no view gets one**, in the background, and keeps
  it until the user closes it. Today such a row vanishes the moment the
  chat stops working, which is the same "a row left on its own" defect.
  The writer runs only on a not-working → working transition observed by
  the stream for a chat with no pane, so a reconnect seed or a delta burst
  cannot mint duplicates.
- **Runner-following has one writer.** Today both the stream's
  `followRunner` and an effect in `agent-chat-pane.tsx` (line 552) can move
  a pane onto a different chat. The stream keeps `retargetPane`; the pane
  effect is reduced to `setPaneRunner` for its own chat. The stream is the
  one that also works when no pane is mounted.

## Rendering and performance

- **Switching views is a pointer write.** The renderer draws
  `views[activeViewId].layout` (or `stage`). No tree is moved between
  `rootLayout` and `parkedViews`, so there is no parking code path. Parked
  views stay mounted and suspended exactly as today, now by mapping over
  records.
- **The band stops deriving.** `recentsForProject` and `space-scroller.tsx`'s
  custom-equality recompute walk every pane, every mounted workspace store
  and the home tree on each change. They are replaced by one memoized
  selector over `views` and `viewOrder` for the project's row ids. Each row
  subscribes to its own labels with narrow selectors: `activeViewId === id`
  for showing, and each member chat's `working` flag from its own workspace
  store. A streaming delta or a working toggle re-renders one row, not the
  band.
- **Chat → pane lookup is O(1).** A memoized index over `panes` (keyed on the
  map's reference) replaces the `Object.values(panes).find(...)` scans in
  every reveal path.
- **Retention reads records.** `workspacesWithViewChat` and
  `useViewWorkspaceIds` resolve owners from the records' panes, with no
  Recents derivation involved.
- **Persistence** keeps the existing 300ms debounce. The payload carries
  `views`, `viewOrder`, `activeViewId`, `activeViewByProject`, `stage`,
  `panes`, `bottomLayout` and `buffers`, and the shallow-compare guard lists
  exactly those. A payload without `views` hydrates to an empty band. No
  compatibility readers.

Resolving each member chat's owning workspace for rendering is unchanged
(`resolveChatOwnerWorkspaceId` / the sidebar chat index), but done per row
instead of in one project-wide walk.

## Consumers to migrate

Every current reader of the removed fields, from `grep`:

- `features/panes/stores/slices/pane-slice.ts`, `window-pane-store.ts`
- `features/panes/lib/pane-views.ts`, `features/panes/utils/pane-command-actions.ts`,
  `features/panes/components/pane-container.tsx`,
  `features/panes/hooks/use-chat-workspace-id.ts`, `features/panes/types/recents-entry.ts`
- `features/agent/lib/open-agent-chat.ts`, `features/agent/components/agent-chat-pane.tsx`
- `features/workspace/stores/hooks/use-workspace-agent-chats-stream.ts`,
  `features/workspace/lib/keep-alive-policy.ts`
- `components/sidebar/lib/recents-entries.ts` (deleted),
  `recents-for-project.ts`, `recents-actions.ts`, `drop-actions.ts`,
  `recents-band.tsx`, `space-scroller.tsx`
- `lib/persistence/schemas.ts`, `lib/persistence/hydrate.ts`
- every `setPaneChat` caller (10 sites), each re-routed to the action above
  that names its intent

The implementation plan starts by re-running that survey, since other
branches are landing in this area.

## Delivery

One change, not staged. Stages would mean running the old and new
existence models side by side, and a second source of truth is the bug this
removes. The work is ordered so each step compiles and its tests pass:
`view-ops.ts` with its invariant checker first, then the slice on top of
it, then consumers, then deleting the old fields.

## Tests

In `web/src/__tests__/` mirroring source paths. Targeted runs only.

1. **`view-ops.test.ts`**: each helper against the invariants, including
   the edge each one owns (the last chat leaving a view, the stage being
   promoted, a merge emptying its source view).
2. **Sequence property test**: a seeded, deterministic generator applies
   random sequences of every action in the table plus every non-row
   event (keep-alive eviction, workspace unmount, project switch, reconnect
   seed, hydrate round-trip). After each step: `assertViewIntegrity` holds,
   and the set of rows changed only if the step was a row-changing action.
   Seeds are fixed, so failures reproduce.
3. **Regression tests**:
   - create a branch while a project-home chat is showing → both rows exist.
   - the same with a repo chat.
   - a working chat whose workspace store is unmounted keeps its row.
   - `/clear` in a grouped view → same row, same group, new conversation.
4. **Existing Recents and drop tests** keep their user-visible assertions.
   Tests that asserted on `dormantArrangements` or `'dormant'` rows are
   rewritten to the behaviour this spec defines, and each one is named in the
   plan.
5. **Live verification** on `make dev-desktop` through the Tauri bridge:
   the branch repro, merge and detach, close, reorder, project switch, and a
   reload preserving the band.
