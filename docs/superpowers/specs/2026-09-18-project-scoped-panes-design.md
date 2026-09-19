# Project-scoped panes — what is on screen belongs to the space you are in

**Date:** 2026-09-18

**Status:** APPROVED and IMPLEMENTED (frontend only, zero Go changes, as §7 promised).
§10's open questions were ruled on by the product owner before implementation — see
§10 itself, where each now carries its answer. Two of them (the bottom panel, Q7, and
reopen-closed-tab, Q8) were deliberately scoped OUT of this pass and remain global,
unscoped window-level state; they are the only parts of the content area this
partition does not reach.

Written to be read and ruled on before any implementation starts, the same way
[`2026-09-18-restyling-v2-remediation-plan.md`](./2026-09-18-restyling-v2-remediation-plan.md)
was written before its own fixes were built.

**The request, verbatim:**

> "Whats on the main content pane, either views or a group of views, should be kept
> inside that project. I shouldn't be able to move an view into another project, and
> I shouldn't be able to still see a chat that is inside another project when moving
> into a different one. This should work exactly like Zen Browser's Spaces and tabs
> work."

**Scope:** which project a *view* belongs to, and what the content area shows when
the active project changes. Touches
`web/src/features/panes/stores/slices/pane-slice.ts`,
`web/src/features/panes/stores/window-pane-store.ts`,
`web/src/lib/persistence/schemas.ts` + `workspace-layout.ts` + `hydrate.ts`,
`web/src/components/layout/ide-shell.tsx`,
`web/src/components/sidebar/lib/drop-actions.ts`, and
`web/src/features/panes/utils/pane-command-actions.ts`.

**Not in scope, and deliberately so:** the backend. **Nothing in this proposal needs
a single Go change.** A chat's project is already single-valued and already
resolvable client-side (§3.5); this is a frontend partition of an existing
frontend structure. Also out of scope: the sidebar tree (already partitioned —
§3.1), Recents' own contents (already filtered — §3.3), multi-window (§10.6), and
the three unrelated bugs batched with this request (the rename modal, the
cross-project folder leak, Recents reorder).

**Evidence status.** Everything in §2 and §3 was read out of source: Zen's tree at
`/Users/char2cs/Projects/Cloned/desktop/src/zen`, Crowbar's at this worktree's
`web/src`. **Nothing here was driven through the running app** — this investigation
was read-only by brief. §3.4 marks each baseline claim with the code that proves it
and names the one-gesture repro that would confirm it live.

---

## 1. The three symptoms, and what they have in common

| # | Symptom | Status |
|---|---|---|
| 1 | Switch from project A to project B; A's panes are still on screen, A's chats still rendering | Code-confirmed (§3.2). This is the user's "I shouldn't be able to still see a chat that is inside another project". |
| 2 | A drop can merge a chat from one project into a view holding another project's chat | Code-confirmed (§3.4). This is "I shouldn't be able to move a view into another project". |
| 3 | A view has no project at all, so nothing can even be asked "which space is this?" | Code-confirmed (§3.2). This is the root cause of 1 and 2. |

One cause: **panes are window-level and projects are not a fact panes carry.**
Task 26 deliberately made the pane store one-per-window
(`window-pane-store.ts:29-43`) to fix a real bug — the old per-workspace registry
destroyed a live layout whenever its workspace aged out. That was correct and must
not be undone. But "one store for the window" was read as "one *screen* for the
window", and the screen is where the space boundary belongs.

---

## 2. How Zen actually does it

Read from `src/zen/spaces/`, `src/zen/drag-and-drop/`, `src/zen/space-routing/`.

### 2.1 It is a HARD PARTITION, tagged and structural — not a filter

A tab carries `zen-workspace-id` as a real attribute, and it is **reparented into
its space's own container**: `moveTabsToWorkspace` sets the attribute and calls
`container.insertBefore(tab, insertElement)` where `container` is that space's
`tabsContainer`/`pinnedTabsContainer` (`ZenSpaceManager.mjs:1495-1536`). Every space
is its own `<zen-workspace>` custom element owning those two containers
(`ZenSpace.mjs:49-130`, `261-262`).

Nothing derives a tab's space from its content. `_shouldShowTab`
(`ZenSpaceManager.mjs:2321-2365`) is a one-line comparison at the end —
`return tabWorkspaceId === workspaceUuid` — after handling the exceptions.

### 2.2 Switching hides; it never unmounts

The inactive space's whole subtree stays in the DOM and is hidden visually only:

```css
:root:not(:is([animating-background], [swipe-gesture])) #navigator-toolbox:not([movingtab]) &:not([active]) {
  -moz-subtree-hidden-only-visually: 1;
}
```
(`spaces/zen-workspaces.css:401-403`)

Tabs of inactive spaces stay loaded. Unloading them is an **explicit user action**,
not a consequence of switching (`unloadWorkspace` / `unloadAllOtherWorkspaces`,
`ZenSpaceManager.mjs:1455-1489`).

Note the `#navigator-toolbox:not([movingtab])` clause: while a tab is being dragged,
the other spaces are *not* hidden. The partition relaxes exactly for the one gesture
that crosses it.

### 2.3 Crossing a space is a deliberate, explicit gesture

Three ways, all of them opt-in:

- **Drag onto a space icon** → spring-loaded switch. `handle_spaceIconDragOver`
  calls `changeWorkspaceWithID(spaceId)` mid-drag
  (`ZenDragAndDrop.js:749-767`); dragging near the sidebar's left/right edge does the
  same after `zen.tabs.dnd-switch-space-delay`
  (`ZenDragAndDrop.js:716-746`).
- **Drop, then reassign.** `#handle_dropSwitchSpace` retags the dragged tab(s) to
  **whatever space is now active** and selects it (`ZenDragAndDrop.js:972-1000`).
  The tab is *moved*, never shared.
- **A menu.** "Move tab to space" builds a menuitem per space and calls
  `moveTabsToSyncedWorkspace` (`ZenSpace.mjs:428-458`).

### 2.4 Selecting content follows content — the space follows the tab, not the reverse

`onBeforeTabSelect`: selecting a tab whose `zen-workspace-id` is not the active
space **switches the space** and remembers that tab as the space's last selected
(`ZenSpaceManager.mjs:2299-2318`). Space routing does the same for URLs: navigating
to a domain routed to another space moves the tab there and switches
(`common/sys/ui/ZenSpaceRoutingNavigation.sys.mjs:75-113`).

### 2.5 Untagged content is ADOPTED by the active space

`_shouldShowTab`'s own migration rule:

```js
if (!tabWorkspaceId) {
  // Assign workspace ID to tabs without one
  this.moveTabToWorkspace(tab, workspaceUuid);
  return true;
}
```
(`ZenSpaceManager.mjs:2356-2361`; same rule in `onTabBrowserInserted`, `:2825-2831`,
and `#prepareNewWorkspace`, `:1539-1553`.)

This is Zen's entire compat story for tabs that predate spaces. It is one branch,
not a migration.

### 2.6 There is exactly one deliberate exception: essentials

"Essential" tabs are shared across every space, optionally narrowed by container
(`_shouldShowTab`'s `isEssential` branch, `:2337-2353`). The exception exists, it is
named, and it is the only one.

### 2.7 A space owns its content's lifetime

Deleting a space closes its tabs (`#deleteWorkspaceOwnedTabs`, `:1443-1453`). Each
space has a remembered last-selected tab (`lastSelectedWorkspaceTabs`) and gets its
own empty tab when it has nothing (`selectEmptyTab`, `_handleTabSelection`,
`:2366-2412`).

### 2.8 Summary of the model worth copying

| Zen | Mechanism |
|---|---|
| A tab belongs to one space | a **tag** on the tab + structural parentage, never derived |
| Switching space | hide visually, keep alive, `inert`-equivalent |
| Crossing a space | explicit gesture only; the content is **moved**, not shared |
| Clicking foreign content | switches the space to it |
| Untagged content | adopted by the active space |
| Shared content | one named exception (essentials) |

---

## 3. Crowbar today — the baseline, with evidence

### 3.1 The SIDEBAR is already partitioned, and correctly

`SpaceScroller` renders **one snap panel per project**, and the active panel is the
active project (`components/sidebar/space-scroller.tsx:432-560`). Tree drops already
refuse to cross a project: `sidebar-drop-policy.ts` compares `projectId` at four
separate call sites (`:313-322`, `:349-352`, `:389`). The sidebar is not the problem
and needs no change.

### 3.2 The CONTENT AREA is one tree for the whole window, with no project in it

- `windowPaneStore` is a module-level singleton, created once, never destroyed
  (`features/panes/stores/window-pane-store.ts:117-121`).
- `WindowPaneSurface` renders the pane tree **once**, as a sibling of the workspace
  slots, outside all of them (`features/workspace/components/workspace-host.tsx:341-382`).
  Its own doc says why: rendering it inside each retained `WorkspaceView` drew N
  copies and cost 150-400ms per switch.
- `rootLayout` is the **showing view's** tree; every other open view's tree sits in
  `parkedViews` (`pane-slice.ts:196-231`).
- **`grep -rn projectId web/src/features/panes/` returns three hits, all of them in
  a comment or an unrelated home-tree lookup.** The pane system has no concept of a
  project at all.

So: navigating from `/ide/A/home` to `/ide/B/home` changes the route, the sidebar
panel, and the ambient workspace store — and leaves `rootLayout` exactly as it was.
The panes of project A keep rendering. **That is symptom 1, and it is structural,
not a bug in a handler.**

### 3.3 Recents is ALREADY a filtered view, and it shows the cost of filtering

`recentsForProject` (`components/sidebar/lib/recents-for-project.ts`) filters the
one window-level pane store down to one project per band. To do that it must first
answer "which project is this chat in?", and that answer takes three seeded sources
plus an overlay:

1. every project repo's `chats` (`chat.workspaceId`),
2. the project's home tree,
3. every *live* workspace store's `agentChats`,

with an explicit comment that seeding from the sidebar first is what makes a
persisted dormant entry survive a reload at all, "its chat's workspace store is not
mounted until someone opens it again, and reading only the live registry left the
band empty until then" (`:95-103`).

It also documents the exact thing this proposal exists to stop:

> "a wrong PROJECT is still a real thing to guard against; **two panes on screen can
> legitimately belong to different projects' workspaces at once**." (`:64-70`)

### 3.4 Exactly how far the leak reaches

| Question | Answer | Evidence |
|---|---|---|
| Can a pane opened under project A still show while B is active? | **Yes.** | `rootLayout` is never touched by a project change; `WindowPaneSurface` is mounted on every `/ide/*` route including `home` (`workspace-host.tsx:341`, `ide-shell-workspace-content.tsx`). |
| Can a chat from B be dropped into a view holding A's chat? | **Yes.** | `performSidebarPaneDrop` → `openChatIntoPane` (`drop-actions.ts:905-913`); the whole path resolves a chat's *workspace* and never its project. `paneChatSubject` (`:884-897`) has no project check. |
| Can one view hold panes from two projects? | **Yes**, and it is documented as legitimate. | `recents-for-project.ts:64-70`; `pane-container.tsx:136-160` exists precisely so a pane can render a chat from a workspace other than the ambient one. |
| Can a Recents entry from another project show while B is active? | **No** — the band is per project. | `recents-for-project.ts`. But the *pane* it points at is on screen anyway, which is the leak. |
| Does clicking a row already switch the project? | **Yes.** | `space-content-actions.ts:224-330` navigates to `/ide/$projectId/$repoId/$wsId` before opening. Crowbar already has Zen's §2.4 behaviour, in the one direction it has it. |

**The one-gesture live repro** (for whoever verifies this, which has not happened):
open a chat into a pane under project A, then click project B's mark in the sidebar
footer (or ⌘2). A's chat is still on screen.

### 3.5 What a project's boundary already is, in the data

A chat resolves to exactly one project, and cannot resolve to two:

`chat.WorkspaceID` → a workspace → exactly one repo → `repo.projectId` (single-valued,
`lib/store/sidebar.ts:228`). Project-home chats resolve through the project's own
home workspace (`home-workspace-resolver.ts`, keyed by project id). Folders now
carry `HomeID` and `InHome` compares it (`api/internal/domain/folder.go:13-22`) —
tonight's own fix. Editor buffers carry `workspaceId`
(`buffer-slice.ts:105-175`), so they resolve the same way.

**A chat is in one project. There is no shared-across-projects case in the data
model** — which is what makes a hard partition honest rather than a lie told over a
shared pool (see §10.3, which asks the user to confirm this is the intent).

### 3.6 The half of Zen's model Crowbar has already built

`SplitViewRoot` renders **every** view — showing and parked — in one list keyed by
view id, flipping only `showing`, a `style` and `inert`
(`features/panes/components/split-view-root.tsx:29-53`, `76-114`):

```js
const PARKED_VIEW_STYLE = { display: 'none' }
```

with a hard-learned warning attached: rendering the showing view in one place and
the parked ones in another moves a subtree between parents, which React can only do
by unmounting — measured live as a parked view's shell terminal being destroyed and
never coming back.

That is `-moz-subtree-hidden-only-visually` (§2.2), already built, already
load-bearing. **The mechanism is there. Only the eligibility rule is missing.**

---

## 4. The laws

Seven. Everything in §6 is a consequence of one of them, and breaking one is a
change to this design, not to the code.

1. **A VIEW BELONGS TO EXACTLY ONE PROJECT.** The project is a **tagged fact on the
   view**, written when the view is minted. It is never re-derived from the view's
   contents in a render path.
2. **THE SCREEN SHOWS ONE PROJECT'S VIEW.** `activeViewId` always names a view of
   the active project. Every other view — this project's and every other project's —
   is parked.
3. **PARKED IS NOT CLOSED.** Switching project parks; it never closes, never unmounts,
   never stops a CLI. Only `closePane`/`closeView` end a view. (This is
   `2026-08-28`'s law 5, unchanged, extended to a project switch.)
4. **CONTENT NEVER CROSSES A PROJECT.** No drop, no click, no command may put chat X
   into a view of a project that is not X's. The user's own words: "I shouldn't be
   able to move a view into another project."
5. **CLICKING FOREIGN CONTENT SWITCHES THE PROJECT.** Reaching a chat that lives in
   another project is a space switch, never a leak into the current space. (Zen §2.4;
   Crowbar's `navigateThenOpenChat` is already this.)
6. **AN EMPTY PANE BELONGS TO NO PROJECT.** The empty stage holds no chat, so it
   leaks nothing, so it needs no tag and may be shown by any project.
7. **NO BACKEND, NO MIGRATION.** The partition is a frontend fact over data that
   already answers the question. An untagged view is **adopted**, per §8 — no
   backfill, no dual read, graceful fallback only.

---

## 5. Shape of the whole thing

**Before** — one screen, no project in it:

```
┌ window ───────────────────────────────────────────────────────────────┐
│ sidebar                              │ content area                    │
│ ┌ SpaceScroller ────────────────┐    │ ┌ WindowPaneSurface ─────────┐  │
│ │ ▸ panel: project A  ← snapped │    │ │ view v1  showing    (A)    │  │
│ │   panel: project B            │    │ │ view v2  parked     (A)    │  │
│ │   panel: project C            │    │ │ view v3  parked     (B) ✱  │  │
│ └───────────────────────────────┘    │ └────────────────────────────┘  │
│   Recents: filtered to A  ✓          │   activeViewId = v1             │
└───────────────────────────────────────────────────────────────────────┘
  ✱ nothing stops v3 becoming the showing view while A is active,
    and nothing stops a B chat being dropped into v1.
```

**After** — the same structure, with one tag and one rule:

```
                        activeProjectId = A            (written once, from the route)
                                │
     viewProjects   v1→A  v2→A  v3→B                   (the tag, law 1)
     activeViewByProject   A→v1   B→v3                 (Zen's lastSelectedWorkspaceTabs)
                                │
┌ WindowPaneSurface ────────────┼───────────────────────────────────────┐
│  view v1  showing    (A)   ←──┘  activeViewId, always a view of A     │
│  view v2  parked     (A)         display:none + inert, alive          │
│  view v3  parked     (B)         display:none + inert, alive          │
└───────────────────────────────────────────────────────────────────────┘

  switch to B:   park v1 under A ──▶ activeViewId = activeViewByProject[B] = v3
  switch to C (never opened):       activeViewId = the shared empty stage (law 6)
```

**The whole on-screen partition falls out of law 2.** `SplitViewRoot` needs no
change at all: it already renders exactly one view as `showing` and every other one
parked-but-alive (§3.6). Making `activeViewId` always belong to the active project
makes "you can only see this project's content" a property of the data the renderer
already walks — the same argument `rootLayout`'s own doc makes about views
(`pane-slice.ts:198-210`).

---

## 6. The recommendation: HARD PARTITION, at the VIEW, by a stamped tag

### 6.1 Hard partition, not a filtered view

A filtered view would mean: keep one pool, and at render time ask every pane "which
project are you?", showing only the matches. **Recommend against, for four reasons,
three of them measured in this codebase already.**

1. **The derivation is not always answerable, and its failure mode is a blank
   screen.** A pane's project is `chat → workspace → repo → project`. Right after a
   reload the owning workspace store is not mounted; `recents-for-project.ts:95-103`
   had to seed from three separate sources precisely because of this, and its failure
   mode is a missing *row*. A filter in the pane path has the same failure with a
   much worse symptom: the user's whole screen goes empty for a frame, or forever.
2. **Law 6's case has no answer at all.** An empty stage / new-tab pane holds no
   chat. A filter cannot classify it; a tag simply is not required for it.
3. **This codebase has already made this exact call once, and written down why.**
   `viewId` exists because the tiling tree "cannot tell 'these are one view' from
   'these are two views side by side'" — the resolution was to **tag the grouping
   fact on the pane**, not to infer it (`types/pane.ts:27-47`). "Which project" is
   the same shape of question one level up.
4. **Zen tags. It does not filter.** `zen-workspace-id` is an attribute, set on
   creation and on every deliberate move (§2.1).

A filter is also *slower* in the one place that matters: a per-frame
chat→workspace→repo walk for every pane, in the render path, on a surface whose own
history is full of "this re-rendered every pane in the window" regressions
(`pane-container.tsx:126-131`, `split-view-root.tsx:56-59`).

### 6.2 At the VIEW, not at the pane

Panes inside one view were merged *deliberately* — the only gesture that does it is
a drag-and-drop (`types/pane.ts:29-36`). A view is therefore the smallest unit the
user ever thinks of as "a thing on my screen", and it is already the unit that
parks, activates and closes whole. Tagging views means:

- one tag per user-visible arrangement, not one per leaf;
- `activateView` is already the single write path for `activeViewId`, and its own
  doc states that **every** other way of reaching a view — a Recents row click,
  revealing an already-open chat, a drop — arrives through `setActivePane`, which
  calls it (`pane-slice.ts:68-73`). Law 2 therefore has exactly one place to be
  enforced, and no bypass;
- a merge (`mergePaneIntoView`) inherits the target view's project, and law 4 turns
  into a single refusal at one call site.

### 6.3 The state this adds

Two fields on `PaneSlice`, both persisted:

```ts
/** Law 1: the project a view belongs to, written when the view is minted,
 *  never re-derived. A view id absent here is untagged — see §8. */
viewProjects: Record<string, string>

/** Zen's `lastSelectedWorkspaceTabs`: which view each project was last showing,
 *  so switching back is a return, not a reset. */
activeViewByProject: Record<string, string>
```

plus one field the store needs to know where "now" is:

```ts
/** The project the screen is currently in. ONE writer: the same effect in
 *  ide-shell.tsx that already syncs `useProjectStore.activeProjectId` from the
 *  route (ide-shell.tsx:206-214). Never read from the route in a render path. */
activeProjectId: string | null
```

### 6.4 The actions

| Action | Change |
|---|---|
| `setActiveProject(projectId)` | **New.** Park `rootLayout` under the current `activeViewId`; set `activeProjectId`; activate `activeViewByProject[projectId]` if it still exists, else the shared empty stage (law 6). This is Zen's `#performWorkspaceChange` + `_handleTabSelection`. |
| `addPane` | Stamp `viewProjects[newViewId] = activeProjectId`. |
| `splitPane` / `detachPaneToOwnView` | New view inherits the source view's project (never the active one — a split of a parked view must not change its space). |
| `mergePaneIntoView` | Inherits the **target** view's project; refuses if the incoming chat's project differs (law 4). |
| `activateView(viewId)` | Enforce law 2: if the view's project ≠ `activeProjectId`, this is a cross-project reveal and must be routed (law 5), not silently activated. Record `activeViewByProject[project] = viewId`. |
| `closeView` / `closePane` | Drop the view's `viewProjects` entry and any `activeViewByProject` pointer to it. |

### 6.5 The refusals (law 4), all at existing call sites

- `openChatIntoPane` (`drop-actions.ts`): refuse when the subject chat's project ≠
  the target view's project. Belt-and-braces — after §6.4 the geometry already makes
  it nearly unreachable, because the only rows draggable are the active project's
  panel's and the only pane tree on screen is the active project's.
- `openChatIdInOwnView` (`pane-command-actions.ts`): a chat already up in another
  project's parked view is a **route + switch** (law 5), not a reveal in place.

---

## 7. What stays exactly the same

- **The backend.** Zero Go changes. No new field, no endpoint, no migration.
- **`SplitViewRoot`** (§5) — and it must stay the same: never reorganise the
  one-list-keyed-by-view-id shape, or parked terminals die
  (`split-view-root.tsx:29-45`).
- **`WorkspaceHost` retention.** `viewWsIds` already spans projects; parked views of
  inactive projects keep their workspaces mounted exactly as parked views of the
  active project do today. This proposal adds no memory (see §10.4 for whether it
  should *remove* some).
- **`pane-chat-workspace.ts`.** A pane still resolves its chat's own workspace; that
  stays the right answer within a project.
- **The sidebar tree and its drop policy** (§3.1).
- **Recents' contents.** The per-project filter keeps working and becomes redundant
  defense in depth; it should not be deleted in the same change that relies on it.

---

## 8. Migration and compat

The house rule is **no legacy migration, graceful fallback only** — so this is one
branch at hydrate, Zen's own §2.5 adoption rule, not a backfill.

**Panes already open when this ships / any persisted layout.** The persisted
`WorkspaceLayout` (one IndexedDB row, key `WINDOW_SESSION_ID = 'window'`,
`lib/persistence/workspace-layout.ts:12-25`) carries no `viewProjects`. At hydrate,
for each view:

1. resolve the project from the view's panes' chats, **once**, not per render — the
   entity cache is cross-project and every known project's repos and home tree are
   already streamed (`lib/store/project-visibility.ts:60-67`,
   `components/app-sync-engine.ts:547-566`), so this is usually answerable offline;
2. unresolvable (no chat, an evicted chat, an empty stage) → **adopt into the project
   active at hydrate**, exactly as `_shouldShowTab` adopts an untagged tab
   (§2.5).

A mis-filed view is recoverable by the user in one gesture and costs nothing; a
crashed hydrate is not. Prefer adoption to refusal everywhere here.

**Dormant Recents entries.** `dormantArrangements` holds chat ids only
(`pane-slice.ts:231-245`). Their project is derived today by `recentsForProject` and
should keep being derived — a dormant entry has no view to tag. Going forward, stamp
`projectId` on `RecentsEntry` at mint with the derivation as fallback.

**A project deleted while it has views.** Zen closes a deleted space's tabs
(§2.7). Crowbar already has `handleTrashProject`
(`space-content-actions.ts:403`); its views must be closed through the real
`closeView` teardown, never orphaned in `parkedViews` where nothing can reach them.
This is a required part of the change, not a follow-up.

---

## 9. Traps for whoever implements this

1. **Do not move a view's DOM between parents.** `split-view-root.tsx:29-45`
   documents a live-measured terminal death from exactly that. Any project-eligibility
   change must stay a `showing` flag on one stable, view-id-keyed list.
2. **Do not derive a project in a render path.** §6.1; and note that `PaneContainer`
   and `SplitViewRoot` both carry their own "this re-rendered every pane in the
   window" scars.
3. **`ROOT_PANE_ID` is both a pane id and a view id**, fixed and shared
   (`makeRootLeaf`, `pane-slice.ts:254-266`; read back by
   `selectIsShowingEmptyStage`, `:426`). A naive "one root pane per
   project" breaks the empty-stage check and the persisted layout. Law 6 exists partly
   to avoid needing one.
4. **`activeProjectId` must have one writer.** Two (the route and a click handler) is
   how the sidebar's own project-switch bugs happened; `ide-shell.tsx:206-214` is
   already the one place the route becomes state.
5. **Persist the two new fields** in the same debounced subscription and the same
   shallow-compare guard as the rest (`window-pane-store.ts:59-102`) — a field left
   out of that comparison silently never persists.

---

## 10. Open questions — ANSWERED by the product owner, 2026-09-18

Each question's ruling is recorded inline below the question. Nothing here is still
open; the implementation was built to these answers.

1. **ANSWERED: YES**, per this proposal's own law 3 — park, never close.
2. **ANSWERED: NO** such gesture exists or should be added. Matches the request verbatim.
3. **ANSWERED: OUT OF SCOPE.** Current import behaviour is unchanged. Each `Repo` row's
   chats still resolve to exactly one project, so law 1 holds regardless.
4. **ANSWERED: YES, unchanged** — inactive parked views stay warm, exactly as §7 says
   ("adds no memory"). No unload verb, no auto-eviction.
5. **ANSWERED: the ONE shared empty stage** (law 6), not a per-project one.
6. **ANSWERED: do not design for it.** `activeProjectId` is a single value matching
   today's single-window reality. A deliberate scope limit.
7. **ANSWERED: OUT OF SCOPE for this pass.** `bottomLayout`/`BOTTOM_PANE_ID` stays a
   global, unscoped drawer. Ask for it explicitly if the drawer should be per project.
8. **ANSWERED: OUT OF SCOPE for this pass.** `closedBuffersHistory` stays one flat
   window-level list; ⌘⇧T can still resurrect a buffer from the project you left.

1. **Does switching project keep the other project's chats running?** This proposal
   says yes (law 3: park, never close — a CLI keeps working and its Recents row stays
   in its own project's band). Zen agrees (§2.2). Confirm — the alternative ("leaving a
   space stops its work") is a different product.
2. **Is there any gesture to move a view to another project at all?** Zen has one
   (drag onto the space icon, §2.3). Your words say there should not be one, and the
   data model agrees — a chat's project comes from its workspace's repo, so "moving
   the view" without re-filing the chat would be a lie. **Recommendation: no such
   gesture.** Confirm, because if you *do* want one, it is a backend re-file, not a
   frontend move, and that is a much larger piece of work.
3. **Can the same chat legitimately be in two projects?** Checked: **no** — §3.5. One
   caveat to rule on: the same git repo *imported into two projects* produces two
   `Repo` rows and therefore two separate sets of chats. Is that intended, or should
   an import into a second project be refused?
4. **Do inactive projects' parked views stay warm forever?** Today every workspace
   with a chat in Recents stays mounted, across projects
   (`workspace-host.tsx`'s `viewWsIds`). Zen makes unloading an explicit action
   (`unloadAllOtherWorkspaces`, §2.2). Should Crowbar get the same explicit "unload
   this space" / "unload every other space" verb, or an automatic eviction for
   non-active projects?
5. **What does a project with no view ever opened show?** Proposal: the one shared
   empty stage (law 6). The alternative is a per-project empty stage remembering its
   own new-tab state. Zen mints one empty tab per space (§2.7).
6. **Multi-window.** The persisted layout is a single fixed row (`'window'`,
   `workspace-layout.ts:12`) and `2026-07-24-multi-window-design.md` is "approved for
   implementation". If two windows can be open, is the active project per window (it
   must be) — and does that need the persisted layout keyed per window before this
   ships, or after?
7. **The bottom panel** (`bottomLayout`, a second tree with its own fixed
   `BOTTOM_PANE_ID`) — project-scoped like the main area, or a global drawer that
   survives a project switch?
8. **Reopen-closed-tab across a switch.** `closedBuffersHistory` is one flat
   window-level list. Should ⌘⇧T after a project switch be able to resurrect a buffer
   from the project you left, or should that history be per project too?
