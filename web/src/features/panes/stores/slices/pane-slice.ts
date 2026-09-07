import type { StateCreator } from 'zustand'
import type { WindowPaneState } from '../window-pane-store.types'
import { isEditorContent, type EditorTabBase } from '@/features/panes/types/pane-content'
import { fileUri } from '@/features/editor/lib/editor-uri'
import { ROOT_PANE_ID, BOTTOM_PANE_ID } from '@/features/panes/constants/pane'
import type {
  PaneGroup,
  LayoutNode,
  SplitDirection,
  SplitPlacement,
} from '@/features/panes/types/pane'
import type { RecentsEntry } from '@/features/panes/types/recents-entry'
import {
  createLeaf,
  splitLayout,
  closeLayout,
  findLeaf,
  findSplit,
  getAllLeafIds,
  getFirstLeafId,
  distributeSplit,
  resizeFlattenedLayout,
  normalizeLayout,
  getAdjacentLeafId,
} from '@/features/panes/utils/pane-layout'
import { syncSoleEditorTabCloseability } from './buffer-slice'
import {
  getWorkspaceStore,
  isChatWorking,
} from '@/features/workspace/stores/workspace-store-registry'
import { viewIdOf, viewIsShared } from '@/features/panes/lib/pane-views'
import { releaseClosedChat } from '@/features/panes/lib/release-closed-chat'
import { nanoid } from 'nanoid'

export interface PaneActions {
  /** Spec §8.1: "into THIS view, on that side" — the MERGE. The new pane is
   *  carved out of `paneId`'s own share of the window AND tagged with
   *  `paneId`'s `viewId`, so the two are one view from here on: one Recents
   *  row, one group. Drag-and-drop is the only gesture that reaches this with
   *  a chat (that is the whole rule — see `openChatIntoPane`); the split
   *  commands reach it too, and mean the same thing. */
  splitPane(
    paneId: string,
    direction: SplitDirection,
    bufferId?: string,
    placement?: SplitPlacement,
  ): string | null
  /** Spec §8.4: a CLICK "makes its own view" — a brand-new view that TAKES
   *  THE SCREEN, carrying a `viewId` nothing else shares and a tiling tree of
   *  its own holding one empty pane. Whatever was showing is parked whole
   *  into `parkedViews` (it goes away from view, not away), so the new view
   *  fills the content area rather than tiling beside it.
   *
   *  This used to `appendLeaf` onto the ONE shared `rootLayout`, which is the
   *  bug the view model exists to kill: two independently-clicked chats were
   *  genuinely separate views in the data and still drew side by side,
   *  because a peer leaf in the shared tiling tree is all "a separate view"
   *  ever amounted to. `splitPane` still answers §8.1's different question
   *  ("into this view, on that side") and is the only thing that grows a
   *  view. Returns the new pane's id, empty and active. Root layout only — a
   *  click never opens into the bottom panel. */
  addPane(): string | null
  /**
   * Put `viewId` on screen: its tree becomes `rootLayout`, and the tree that
   * was showing is parked under its own view id. A no-op for the view that
   * is already showing, and for one nothing has parked.
   *
   * THE one write path for `activeViewId`. Every other way of reaching a
   * view — clicking its Recents row, revealing a chat that is already open,
   * dropping a chat onto it — goes through `setActivePane`, which calls this
   * for the pane's owning view before focusing the pane.
   */
  activateView(viewId: string): void
  /**
   * Close a whole VIEW — every pane in it, through `closePane`, one at a
   * time, so each member's own chat gets the full teardown (`stopChat` +
   * workspace eviction; see `release-closed-chat.ts`) rather than only
   * whichever pane the gesture happened to name. What Recents' × means for a
   * view of any size (spec §5.4).
   */
  closeView(viewId: string): void
  /** Make `paneId` a view of ITS OWN — a fresh `viewId` nothing else carries.
   *  A no-op when it already is one (nothing else shares its view), so a
   *  caller can state the guarantee unconditionally without churning the
   *  store. What a CLICK uses when it lands in a pane that already exists:
   *  §8.4's "makes its own view" is a promise about the view, not about
   *  whether a pane had to be created to keep it. */
  detachPaneToOwnView(paneId: string): void
  closePane(paneId: string): void
  setActivePane(paneId: string): void
  activateEditorTabInPane(paneId: string, tabId: string): void
  addEditorTabToPane(paneId: string, tab: EditorTabBase): void
  removeEditorTabFromPane(paneId: string, tabId: string): void
  moveEditorTabToPane(tabId: string, fromPaneId: string, toPaneId: string): void
  setEditorTabPreview(paneId: string, tabId: string): void
  setEditorTabPinned(paneId: string, tabId: string, pinned: boolean): void
  setPaneLocked(paneId: string, locked: boolean): void
  reorderEditorTabs(paneId: string, tabId: string, targetIndex: number): void
  resizePaneSplit(splitId: string, index: number, sizes: [number, number]): void
  distributePaneSplit(splitId: string): void
  togglePaneFullscreen(paneId: string): void
  exitPaneFullscreen(): void
  getAllPaneGroups(): PaneGroup[]
  getPaneById(paneId: string): PaneGroup | null
  getPaneByEditorTabId(tabId: string): PaneGroup | null
  getActivePane(): PaneGroup | null
  clearEditorTabPreviewEverywhere(): void
  switchToNextEditorTab(paneId: string): void
  switchToPreviousEditorTab(paneId: string): void
  navigateToPane(direction: 'left' | 'right' | 'up' | 'down'): void
  /** The one write path for what chat a pane holds. Spec §8.4: swapping a
   *  pane onto a different chat archives whatever it held into
   *  `dormantArrangements` first — nothing a click lands here ever costs
   *  the view that was on screen. */
  setPaneChat(paneId: string, chatId: string | null, runnerId: string | null): void
  /** Spec §5.4: the × on a DORMANT/SET (not-live) Recents entry — "forgets
   *  the arrangement" rather than closing a pane, since there is none to
   *  close. The symmetric removal to `closePane`'s own push onto
   *  `dormantArrangements`. */
  forgetDormantArrangement(entryId: string): void
  /** Spec §9: a chat was DELETED — the one act that removes a thing rather
   *  than a view, and therefore "the only one that can leave a name behind."
   *  Clears the layout of any pane holding it and plucks it from every
   *  arrangement that remembered it, dropping arrangements left empty.
   *  Driven by the daemon's `deleted` frame (and by the reconnect seed's
   *  vanished-chat diff), never by a local close — see
   *  `use-workspace-agent-chats-stream.ts`. */
  forgetChat(chatId: string): void
  /**
   * Spec §8.1: "above / below a Recents entry → it moves to that slot" —
   * moves `entryId` to sit directly before/after `targetId` in the
   * persisted Recents order (`recentsOrder`). Both are ENTRY ids
   * (`RecentsEntry.id` — a pane id, a merged-set nanoid, or a bare chat id
   * for a working-no-view row — never a plain chat id on its own, since a
   * SET's members share one slot), resolved by the caller the same way
   * `deriveRecentsEntries` resolves them.
   *
   * `naturalOrder` is the caller's own current, correctly-derived render
   * order for the band it dragged in (`recentsForProject(...).map(e =>
   * e.id)`) — seeded into the persisted list for any id THIS project has
   * that isn't tracked yet, without disturbing ids some OTHER project
   * already reordered. `recentsOrder` is one flat, window-level ledger —
   * spec §5.6 gives every entry one slot regardless of which project's band
   * is asking — so seeding only ever appends, never reorders what is
   * already there.
   */
  reorderRecentsEntry(
    entryId: string,
    targetId: string,
    mode: 'before' | 'after',
    naturalOrder: readonly string[],
  ): void
}

export interface PaneSlice {
  panes: Record<string, PaneGroup>
  /**
   * THE SHOWING VIEW'S TILING TREE — and only that view's.
   *
   * It used to be the whole window's one tree, holding every open pane at
   * once regardless of which view each belonged to. That is precisely why
   * tagging panes with a `viewId` fixed nothing on screen: the tag said
   * "these are two independent views" while the tree said "tile them beside
   * each other", and the tree is what renders. Now the invariant is that
   * every leaf here belongs to `activeViewId`, so "only the active view
   * occupies the screen" needs no filter in the render path — it is true of
   * the data the renderer already walks, and an inactive view costs the
   * layout exactly nothing.
   */
  rootLayout: LayoutNode
  bottomLayout: LayoutNode
  /**
   * Every OTHER view's tree, keyed by view id — open, off screen, and whole.
   * `rootLayout` and this are disjoint: a view's tree is in exactly one of
   * the two, never both, so there is a single authoritative copy of every
   * arrangement at all times.
   *
   * Parked, NOT closed: the chats in these views keep their vendor CLI, their
   * workspace store and their Recents row. Only `closePane`/`closeView` end a
   * view (and only they run the `releaseClosedChat` teardown).
   */
  parkedViews: Record<string, LayoutNode>
  /** Which view `rootLayout` currently is. Written only by `activateView`
   *  (and by `addPane`, which mints a view and shows it in one step). */
  activeViewId: string
  activePaneId: string
  mostRecentActivePaneIds: string[]
  fullscreenPaneId: string | null
  /**
   * Closed-but-idle views, remembered so the close is undoable — spec §5.5.
   * A chat the daemon is still working keeps its row via `agentChats.working`
   * alone; only an idle close needs to be remembered here.
   *
   * DORMANT ONLY. This used to double as the grouping ledger too — a merge
   * called `groupIntoArrangement` to file both chats under one entry so
   * Recents drew them as one row — which meant "which chats are one view"
   * had two answers that could disagree: this chat-id-keyed list, and the
   * on-screen pane layout, which had no grouping concept at all. `viewId`
   * (types/pane.ts) is now the single grouping fact, tagged on the panes
   * themselves, and `deriveRecentsEntries` reads a LIVE row straight off it.
   * What is left here is the one thing panes genuinely cannot answer: what
   * used to be up and no longer is.
   */
  dormantArrangements: RecentsEntry[]
  /** Recents' own persisted order (spec §5.6/§8.1) — entry ids, written ONLY
   *  by `reorderRecentsEntry`. Empty until the first drag; `deriveRecentsEntries`
   *  falls back to its existing append order for any id not named here. Same
   *  durability as `dormantArrangements` — in-memory for the session, not
   *  written to disk. */
  recentsOrder: string[]
  paneActions: PaneActions
}

function makeRootLeaf(): PaneGroup {
  return {
    id: ROOT_PANE_ID,
    type: 'group',
    chatId: null,
    runnerId: null,
    editorTabIds: [],
    activeEditorTabId: null,
    editorOpen: false,
    viewId: ROOT_PANE_ID,
  }
}

function makeBottomLeaf(): PaneGroup {
  return {
    id: BOTTOM_PANE_ID,
    type: 'group',
    chatId: null,
    runnerId: null,
    editorTabIds: [],
    activeEditorTabId: null,
    editorOpen: false,
    viewId: BOTTOM_PANE_ID,
  }
}

/**
 * Which of the window's tiling trees a pane sits in.
 *
 * There is no longer one root tree: the showing view's is `rootLayout`, the
 * bottom panel's is `bottomLayout`, and every parked view owns one in
 * `parkedViews`. Every action that edits a tree resolves its slot first, so
 * the same code path grows/shrinks a view whether or not it happens to be on
 * screen — which is what lets a chat be dropped onto an off-screen view's
 * Recents row and genuinely land in that view's arrangement.
 */
type TreeSlot = { kind: 'root' } | { kind: 'bottom' } | { kind: 'parked'; viewId: string }

type TreeHolder = Pick<PaneSlice, 'rootLayout' | 'bottomLayout' | 'parkedViews'>

/** Every tree in the window, showing and parked alike. */
function allTreeSlots(state: TreeHolder): TreeSlot[] {
  return [
    { kind: 'root' },
    { kind: 'bottom' },
    ...Object.keys(state.parkedViews).map((viewId) => ({ kind: 'parked' as const, viewId })),
  ]
}

function readTree(state: TreeHolder, slot: TreeSlot): LayoutNode | null {
  if (slot.kind === 'root') return state.rootLayout
  if (slot.kind === 'bottom') return state.bottomLayout
  return state.parkedViews[slot.viewId] ?? null
}

function writeTree(state: TreeHolder, slot: TreeSlot, layout: LayoutNode): void {
  if (slot.kind === 'root') state.rootLayout = layout
  else if (slot.kind === 'bottom') state.bottomLayout = layout
  else state.parkedViews[slot.viewId] = layout
}

/** The slot actually holding `paneId`, or null when no tree does. */
function locatePane(state: TreeHolder, paneId: string): TreeSlot | null {
  for (const slot of allTreeSlots(state)) {
    const tree = readTree(state, slot)
    if (tree && findLeaf(tree, paneId) !== null) return slot
  }
  return null
}

/** {@link locatePane}, falling back to the canonical tree for a pane no tree
 *  holds — the same defaulting `getLayoutKey` did before views existed. */
function paneSlot(state: TreeHolder, paneId: string): TreeSlot {
  return (
    locatePane(state, paneId) ?? (paneId === BOTTOM_PANE_ID ? { kind: 'bottom' } : { kind: 'root' })
  )
}

/**
 * Whether `slot`'s tree is nothing but one pane with nothing in it — the
 * "no view is open" fallback screen (spec §5.4), not a view.
 *
 * It is the one arrangement that must never be PARKED: parking it would file
 * an empty stage in Recents as a view the user could switch back to, and mint
 * a fresh one every time they opened something. It evaporates instead.
 */
function isEmptyStage(state: WindowPaneState, slot: TreeSlot): boolean {
  const tree = readTree(state, slot)
  if (!tree) return false
  const leaves = getAllLeafIds(tree)
  return leaves.length === 1 && isPaneEmpty(state.panes[leaves[0]])
}

/** Take the showing tree off screen — parked under its own view id, or
 *  dropped outright when it is only the empty stage. Leaves `rootLayout`
 *  stale; every caller installs a replacement in the same `set`. */
function parkShowingView(state: WindowPaneState): void {
  if (isEmptyStage(state, { kind: 'root' })) {
    for (const id of getAllLeafIds(state.rootLayout)) {
      delete state.panes[id]
      state.mostRecentActivePaneIds = state.mostRecentActivePaneIds.filter((x) => x !== id)
    }
    return
  }
  state.parkedViews[state.activeViewId] = state.rootLayout
}

/** Put a parked view's tree on screen and focus a pane in it — its most
 *  recently active member, else its first. */
function showParkedView(state: WindowPaneState, viewId: string): void {
  const tree = state.parkedViews[viewId]
  if (!tree) return
  delete state.parkedViews[viewId]
  state.rootLayout = tree
  state.activeViewId = viewId
  const leaves = new Set(getAllLeafIds(tree))
  const next = state.mostRecentActivePaneIds.find((id) => leaves.has(id)) ?? getFirstLeafId(tree)
  state.activePaneId = next
  state.mostRecentActivePaneIds = [
    next,
    ...state.mostRecentActivePaneIds.filter((id) => id !== next),
  ]
}

/** The parked view to fall back to when the showing one ends — most recently
 *  active first, so closing a view reveals the one you were in before it. */
function nextParkedViewId(state: WindowPaneState): string | undefined {
  for (const paneId of state.mostRecentActivePaneIds) {
    const slot = locatePane(state, paneId)
    if (slot?.kind === 'parked') return slot.viewId
  }
  return Object.keys(state.parkedViews)[0]
}

/** Nothing in it at all — no chat, no editor tabs. The one state spec §5.4
 *  calls a fallback rather than a view. */
export function isPaneEmpty(pane: Pick<PaneGroup, 'chatId' | 'editorTabIds'> | undefined): boolean {
  if (!pane) return false
  return pane.chatId === null && pane.editorTabIds.length === 0
}

/**
 * An emptied pane leaves the layout, collapsing into its sibling exactly as
 * closing it would.
 *
 * A pane holding nothing is a FALLBACK — "it should only appear when NO VIEW
 * is opened" — not a view of its own, so it must never sit in a split taking
 * up a share of the window with a wordmark in it and a close button over it.
 * The one legitimate empty pane is the LAST one in its tree, which is the
 * "nothing is open in this window" screen (spec §5.4: "closing the last pane
 * empties it rather than refusing") — that one stays, and `TabBar` draws it
 * without any of the chrome that names or closes pane content.
 *
 * Safe to run as a plain layout edit rather than through `closePane`: an empty
 * pane has no chat to archive into Recents and no editor tabs to hand to a
 * survivor, which is everything `closePane` does beyond the layout itself.
 *
 * Called only from the transitions that actually EMPTY a pane
 * (`setPaneChat(…, null)`, `forgetChat`, `removeEditorTabFromPane`), never on
 * every write — a pane created empty by `splitPane`/`addPane` is filled by its
 * caller in the very next action and must survive the gap.
 */
function dropEmptiedPanes(state: WindowPaneState): void {
  // Every tree, not just the showing one: a pane emptied by an eviction or a
  // deletion is just as much a non-view when it sits in a parked arrangement,
  // and leaving it there would put an empty box in that view the moment the
  // user switched back to it.
  for (const slot of allTreeSlots(state)) {
    const initial = readTree(state, slot)
    if (!initial) continue
    for (const paneId of getAllLeafIds(initial)) {
      // Re-read each pass: an earlier collapse in this loop may have made
      // this the last one standing, or dropped the tree entirely.
      const tree = readTree(state, slot)
      if (!tree) break
      const leaves = getAllLeafIds(tree)
      if (leaves.length <= 1) {
        if (isPaneEmpty(state.panes[leaves[0]])) {
          if (slot.kind === 'parked') {
            // Not a fallback — an off-screen view with nothing in it is
            // nothing at all, so the whole view goes.
            delete state.panes[leaves[0]]
            delete state.parkedViews[slot.viewId]
            state.mostRecentActivePaneIds = state.mostRecentActivePaneIds.filter(
              (id) => id !== leaves[0],
            )
          } else if (slot.kind === 'root') {
            // The showing view emptied out. It is only the "nothing is open"
            // fallback screen if there is genuinely nothing else open —
            // otherwise it is an empty view standing in front of real ones,
            // so it goes and the view behind it comes forward. (Before views
            // there was one tree, so a sole empty leaf could only ever mean
            // the fallback; now it usually doesn't.)
            const reveal = nextParkedViewId(state)
            if (reveal) {
              delete state.panes[leaves[0]]
              state.mostRecentActivePaneIds = state.mostRecentActivePaneIds.filter(
                (id) => id !== leaves[0],
              )
              showParkedView(state, reveal)
            }
          }
        }
        break
      }
      if (!isPaneEmpty(state.panes[paneId])) continue
      const next = closeLayout(tree, paneId)
      if (next === null) break
      writeTree(state, slot, normalizeLayout(next))
      delete state.panes[paneId]
      state.mostRecentActivePaneIds = state.mostRecentActivePaneIds.filter((id) => id !== paneId)
      if (state.fullscreenPaneId === paneId) state.fullscreenPaneId = null
      if (state.activePaneId === paneId) {
        state.activePaneId = getFirstLeafId(readTree(state, slot) ?? state.rootLayout)
      }
    }
  }
}

/**
 * §3.2: "a row with a view is grey" — true when some pane already holds
 * `chatId`. `panes` is a flat `Record` (the split TREE lives only in
 * `rootLayout`/`bottomLayout`, which this doesn't need), so no traversal.
 * Meant to be subscribed per row, the way `sidebar-tree.tsx`'s
 * `SidebarTreeRow` and `recents-band.tsx`'s `RecentsMemberRow` each read
 * their own row's live state — never baked into a `SidebarRow` object ahead
 * of render (see `rows-from-repo.ts`'s own note by `working: false`, which
 * the same latch risk applies to for `hasView`).
 */
export function selectChatHasView(state: Pick<PaneSlice, 'panes'>, chatId: string): boolean {
  return Object.values(state.panes).some((pane) => pane.chatId === chatId)
}

export const createPaneSlice: StateCreator<
  WindowPaneState,
  [['zustand/immer', never]],
  [],
  PaneSlice
> = (set, get) => {
  // Task 26: the Monaco-backed editor manager stays a PER-WORKSPACE resource
  // (attached to each workspace's own `WorkspaceStore`, see workspace-store.ts)
  // even though panes/buffers themselves are now window-level — two retained
  // workspaces (spec: up to RETENTION_CAP=6 at once) can share a relative file
  // path, and giving them one shared Monaco model would silently mix their
  // content. `buf.workspaceId` (set on every buffer, see pane-content.ts) is
  // what resolves which workspace's manager a given tab's model lives on.
  // I3: never CREATE a workspace store just to look up its editor manager —
  // a buffer's owning workspace can already be evicted (buffers outlive
  // their workspace's destroy by design). getOrCreateWorkspaceStore here
  // would silently re-register a store WorkspaceHost never mounted and will
  // never destroy: a leak for the rest of the session.
  const editorManagerFor = (workspaceId: string) => getWorkspaceStore(workspaceId)?.editorManager

  /** Release the held Monaco model for `tabId` in `paneId` (editor tabs only). A
   *  no-op when the tab isn't an editor or the pane didn't hold it (the manager
   *  guards on `held`). Disposes the model when no pane still holds it, so
   *  closing a tab frees its model and a reopen reads fresh content. */
  const releaseEditorTabModel = (paneId: string, tabId: string) => {
    const buf = get().buffers?.find((b) => b.id === tabId)
    if (!buf || !isEditorContent(buf) || !buf.path) return
    editorManagerFor(buf.workspaceId)?.closeBuffer(paneId, fileUri(buf.path))
  }

  return {
    panes: { [ROOT_PANE_ID]: makeRootLeaf(), [BOTTOM_PANE_ID]: makeBottomLeaf() },
    rootLayout: createLeaf(ROOT_PANE_ID),
    bottomLayout: createLeaf(BOTTOM_PANE_ID),
    parkedViews: {},
    // The empty stage IS a view id's worth of state — `makeRootLeaf` tags the
    // root pane with `ROOT_PANE_ID` as its view, so the two agree from boot.
    activeViewId: ROOT_PANE_ID,
    activePaneId: ROOT_PANE_ID,
    mostRecentActivePaneIds: [ROOT_PANE_ID],
    fullscreenPaneId: null,
    dormantArrangements: [],
    recentsOrder: [],

    paneActions: {
      splitPane(paneId, direction, bufferId?, placement = 'after') {
        let newPaneId: string | null = null
        set((state) => {
          const slot = paneSlot(state, paneId)
          const tree = readTree(state, slot)
          if (!tree) return
          const result = splitLayout(tree, paneId, direction, placement)
          if (!result) return
          writeTree(state, slot, result.layout)
          newPaneId = result.newPaneId
          state.panes[newPaneId] = {
            id: newPaneId,
            type: 'group',
            chatId: null,
            runnerId: null,
            editorTabIds: bufferId ? [bufferId] : [],
            activeEditorTabId: bufferId ?? null,
            editorOpen: Boolean(bufferId),
            // A split lands INSIDE the view it was carved from — that is what
            // makes a merge a merge rather than a second view that happens to
            // sit next door. The source pane's view, not a new one.
            viewId: viewIdOf(state.panes[paneId] ?? { id: paneId }),
          }
          // Only when the split landed in the SHOWING tree. A merge into a
          // parked view (a chat dropped on its Recents row) must not point
          // `activePaneId` at a pane no tree on screen holds — the caller
          // routes focus through `setActivePane`, which brings the whole view
          // over first.
          if (slot.kind === 'root') {
            state.activePaneId = newPaneId
          }
          state.mostRecentActivePaneIds = [newPaneId, ...state.mostRecentActivePaneIds]
        })
        return newPaneId
      },

      addPane() {
        let newPaneId: string | null = null
        set((state) => {
          const id = nanoid()
          // What was showing goes away from view, whole and undisturbed —
          // never subdivided to make room, which is what `appendLeaf` used to
          // do here and what made a click read as "appended to what I was
          // looking at".
          parkShowingView(state)
          state.panes[id] = {
            id,
            type: 'group',
            chatId: null,
            runnerId: null,
            editorTabIds: [],
            activeEditorTabId: null,
            editorOpen: false,
            // A BRAND-NEW view. The pane's own id serves as the view id — it
            // was just minted, so nothing else can carry it, and it makes the
            // common "one pane, its own view" case readable in a dump of the
            // store rather than an opaque second nanoid.
            viewId: id,
          }
          state.rootLayout = createLeaf(id)
          state.activeViewId = id
          state.activePaneId = id
          state.mostRecentActivePaneIds = [id, ...state.mostRecentActivePaneIds]
          newPaneId = id
        })
        return newPaneId
      },

      activateView(viewId) {
        set((state) => {
          if (viewId === state.activeViewId) return
          if (!state.parkedViews[viewId]) return
          parkShowingView(state)
          showParkedView(state, viewId)
        })
      },

      closeView(viewId) {
        // One `closePane` per member, deliberately: that action is where the
        // whole teardown lives (Recents bookkeeping, the layout edit, and the
        // `releaseClosedChat` that stops the vendor CLI and evicts the
        // workspace store), and it is what makes closing a MERGED view stop
        // every one of its chats rather than only the pane the gesture named.
        //
        // Members are snapshotted first rather than re-found each pass:
        // closing the last pane of a tree RESEEDS the canonical empty stage
        // under the very same id and view (see `closePane`'s null branch), so
        // a "find the next member" loop would never terminate on it.
        const memberIds = Object.values(get().panes)
          .filter((p) => viewIdOf(p) === viewId)
          .map((p) => p.id)
        for (const paneId of memberIds) {
          if (!get().panes[paneId]) continue
          get().paneActions.closePane(paneId)
        }
      },

      detachPaneToOwnView(paneId) {
        set((state) => {
          if (!state.panes[paneId]) return
          // Already a view of its own — including the untagged case, which
          // `viewIdOf` already reads as independent. Writing anyway would
          // churn `panes` (and with it the layout persistence subscription)
          // on every single click.
          if (!viewIsShared(state.panes, paneId)) return
          const slot = paneSlot(state, paneId)
          const tree = readTree(state, slot)
          if (!tree) return
          // Leaving the group is a move in the LAYOUT too, not just a
          // relabel: the pane has to come out of the tree it shared, or it
          // would keep drawing beside its old view-mates while claiming to be
          // a view of its own — the same split between tag and tree the view
          // model exists to close.
          const remainder = closeLayout(tree, paneId)
          // A FRESH id, never `paneId` itself: a pane that was split OFF of
          // this one carries `viewId === paneId`, so reusing it here would
          // leave the two still grouped.
          const ownViewId = nanoid()
          state.panes[paneId].viewId = ownViewId
          if (remainder === null) {
            // It was the whole tree after all — nothing to move, just retag.
            if (slot.kind === 'parked') {
              delete state.parkedViews[slot.viewId]
              state.parkedViews[ownViewId] = tree
            } else if (slot.kind === 'root') {
              state.activeViewId = ownViewId
            }
            return
          }
          if (slot.kind === 'root') {
            state.parkedViews[state.activeViewId] = normalizeLayout(remainder)
          } else {
            writeTree(state, slot, normalizeLayout(remainder))
          }
          state.rootLayout = createLeaf(paneId)
          state.activeViewId = ownViewId
          state.activePaneId = paneId
        })
      },

      closePane(paneId) {
        // Read BEFORE the layout edit below deletes the pane — the teardown
        // fired at the tail needs to know what this pane was holding.
        const releasedChatId = get().panes[paneId]?.chatId ?? null

        set((state) => {
          const slot = paneSlot(state, paneId)
          const closingPane = state.panes[paneId]
          const closedChatId = closingPane?.chatId ?? null
          // Does the VIEW outlive this pane? Asked while the pane is still in
          // `panes`, so `viewIsShared` can see its own view. A view losing one
          // of several members survives (the rest stay grouped); a view losing
          // its only member is gone, and its id is free for the dormant record
          // below to inherit — which is what keeps a closed view in the same
          // Recents slot it occupied while it was live.
          const viewSurvives = viewIsShared(state.panes, paneId)
          const closedViewId = closingPane ? viewIdOf(closingPane) : paneId

          if (closedChatId) {
            // THIS pane's own view on the chat is ending — spec §8.2's
            // survivor rule applies here too, not only when a chat moves
            // elsewhere: it sheds membership in any MULTI-chat arrangement
            // remembering it, and the survivors keep their slot. Left
            // unstripped, a SET's own `chatIds` never caught up with a pane
            // closed through the tab bar or the pane-close keybinding
            // (never Recents' own × control, which closes every member's
            // pane at once) — the closed chat rode along as "live" forever,
            // since `resolveState` reads an entry live off ANY member still
            // showing.
            const memberOfSet = state.dormantArrangements.some(
              (e) => e.chatIds.length > 1 && e.chatIds.includes(closedChatId),
            )
            if (memberOfSet) {
              state.dormantArrangements = state.dormantArrangements
                .map((e) =>
                  e.chatIds.length > 1 && e.chatIds.includes(closedChatId)
                    ? { ...e, chatIds: e.chatIds.filter((id) => id !== closedChatId) }
                    : e,
                )
                .filter((e) => e.chatIds.length > 0)
            }

            // Spec §5.5: "the view dies, the row does not." A chat the daemon is
            // still working keeps its "working, no view" row off `agentChats.working`
            // alone — nothing to remember yet. An idle chat's view is gone for good
            // unless we remember it here, so the close stays undoable.
            //
            // Skipped when some entry already remembers this chat on its own
            // (checked AFTER the strip above, so a chat just split out of a
            // SET is free to get its own fresh slot here rather than being
            // mistaken for already-remembered).
            const alreadyRemembered = state.dormantArrangements.some((e) =>
              e.chatIds.includes(closedChatId),
            )
            if (!alreadyRemembered && !isChatWorking(closedChatId)) {
              state.dormantArrangements.push({
                // The dead VIEW's id, so the remembered row keeps the slot
                // `recentsOrder` already gave it while it was live (a live row
                // is keyed by its view). Only safe when the view really did
                // die with this pane — a survivor still answers to that id,
                // and two Recents entries sharing one id collide as React
                // keys and in the persisted order alike.
                id: viewSurvives ? nanoid() : closedViewId,
                chatIds: [closedChatId],
                state: 'dormant',
              })
            }
          }

          const tree = readTree(state, slot)
          const result = tree ? closeLayout(tree, paneId) : null
          if (result !== null) {
            writeTree(state, slot, normalizeLayout(result))
            const remainingIds = getAllLeafIds(readTree(state, slot) ?? result)
            const fallbackId =
              remainingIds[0] ?? (slot.kind === 'bottom' ? BOTTOM_PANE_ID : ROOT_PANE_ID)
            if (closingPane) {
              const fp = state.panes[fallbackId]
              if (fp) {
                const existingTabIds = new Set(fp.editorTabIds)
                for (const tabId of closingPane.editorTabIds) {
                  if (existingTabIds.has(tabId)) continue
                  fp.editorTabIds.push(tabId)
                  existingTabIds.add(tabId)
                }
                if (fp.editorTabIds.length > 0) fp.editorOpen = true
                // Only adopt the closing pane's active tab if it actually
                // survived the merge above — otherwise `fp` keeps whichever tab
                // it already had active rather than pointing at an id it
                // doesn't hold.
                if (
                  state.activePaneId === paneId &&
                  closingPane.activeEditorTabId &&
                  fp.editorTabIds.includes(closingPane.activeEditorTabId)
                ) {
                  fp.activeEditorTabId = closingPane.activeEditorTabId
                }
              }
            }
            if (state.activePaneId === paneId) state.activePaneId = fallbackId
            delete state.panes[paneId]
          } else if (slot.kind === 'parked') {
            // The last pane of a view that wasn't even on screen. Nothing to
            // reseed — an off-screen view with no panes is not a fallback
            // stage, it is simply gone.
            delete state.panes[paneId]
            delete state.parkedViews[slot.viewId]
          } else {
            const fallbackId = slot.kind === 'bottom' ? BOTTOM_PANE_ID : ROOT_PANE_ID
            // `paneId` IS `fallbackId` when it was the tree's sole leaf under its
            // own canonical id (the common single-pane-workspace case) — deleting
            // unconditionally below would wipe the fresh empty group this just
            // made. Only a paneId distinct from the fallback is stale.
            if (paneId !== fallbackId) delete state.panes[paneId]
            // The showing view just ended. Another open view takes the screen
            // if there is one — closing a view REVEALS what you had behind
            // it, the way closing a tab does; the empty stage is only what is
            // left when there is genuinely nothing else open.
            const reveal = slot.kind === 'root' ? nextParkedViewId(state) : undefined
            if (reveal) {
              if (paneId === fallbackId) delete state.panes[paneId]
              showParkedView(state, reveal)
            } else {
              state.panes[fallbackId] = slot.kind === 'bottom' ? makeBottomLeaf() : makeRootLeaf()
              writeTree(state, slot, createLeaf(fallbackId))
              if (slot.kind === 'root') state.activeViewId = ROOT_PANE_ID
              if (state.activePaneId === paneId) state.activePaneId = ROOT_PANE_ID
            }
          }
          state.mostRecentActivePaneIds = state.mostRecentActivePaneIds.filter(
            (id) => id !== paneId,
          )
          if (state.fullscreenPaneId === paneId) state.fullscreenPaneId = null
        })

        // "All of Crowbar's chats should die once the user has closed their
        // view... Both. It's like killing a chat tab: removes both out of
        // memory." A closed view was the only thing holding this chat's live
        // session up, on either side — the vendor CLI in the daemon and the
        // owning workspace's in-memory store here. Neither was ever released:
        // `stopChat` (whose own doc says it "is what closing a chat TAB
        // calls") had exactly one caller, the chat view's stop button, and
        // this action only ever did Recents bookkeeping. So a closed chat kept
        // its CLI running and its workspace resident until keep-alive aged it
        // out — or forever, if the workspace stayed the active one.
        //
        // Fired AFTER the layout write, never inside it: the release re-reads
        // panes to decide (a chat still up in another pane is not released at
        // all), and it must see the world with this pane already gone. It is
        // deliberately not awaited — closing a pane is a synchronous, local
        // gesture that must not wait on a round trip — and it never deletes:
        // the chat stays dormant and resumable, its row untouched.
        if (releasedChatId) {
          void releaseClosedChat(releasedChatId, () => get().panes)
        }
      },

      setActivePane(paneId) {
        set((state) => {
          // Focusing a pane means SHOWING it. Every "go to that chat" gesture
          // in the app already routes through here — a Recents row, revealing
          // a chat that is already open, the pane a moved conversation landed
          // in — so making this bring the owning view over is what turns all
          // of them into view switches at once, with no second rule to keep
          // in step. A pane in the showing view resolves to no slot change
          // and this costs nothing.
          const slot = locatePane(state, paneId)
          if (slot?.kind === 'parked') {
            parkShowingView(state)
            showParkedView(state, slot.viewId)
          }
          state.activePaneId = paneId
          state.mostRecentActivePaneIds = [
            paneId,
            ...state.mostRecentActivePaneIds.filter((id) => id !== paneId),
          ]
        })
      },

      activateEditorTabInPane(paneId, tabId) {
        set((state) => {
          const pane = state.panes[paneId]
          if (!pane) return
          // Only a tab the pane actually HOLDS may be activated. A pane resolves
          // its content as `editorTabs.find(t => t.id === activeEditorTabId)`,
          // so an id outside `editorTabIds` draws the empty-pane fallback WITH a
          // populated tab strip above it: tabs visible, none selected, nothing
          // rendered. Same class of defence the old buffer-slice era needed.
          if (!pane.editorTabIds.includes(tabId)) return
          pane.activeEditorTabId = tabId
          state.activePaneId = paneId
          state.mostRecentActivePaneIds = [
            paneId,
            ...state.mostRecentActivePaneIds.filter((id) => id !== paneId),
          ]
        })
      },

      addEditorTabToPane(paneId, tab) {
        set((state) => {
          const pane = state.panes[paneId]
          if (!pane) return
          if (!pane.editorTabIds.includes(tab.id)) pane.editorTabIds.push(tab.id)
          pane.activeEditorTabId = tab.id
          pane.editorOpen = true
          // Sync isUncloseable: the sole editor tab in a pane is uncloseable.
          syncSoleEditorTabCloseability(state, paneId)
        })
      },

      removeEditorTabFromPane(paneId, tabId) {
        // Release this pane's held Monaco model BEFORE mutating state (reads the
        // buffer path, which still exists). Guarded internally: a no-op if the
        // pane never held it.
        if (get().panes[paneId]?.editorTabIds.includes(tabId)) {
          releaseEditorTabModel(paneId, tabId)
        }
        set((state) => {
          const pane = state.panes[paneId]
          if (!pane) return
          const closedIndex = pane.editorTabIds.indexOf(tabId)
          const wasActive = pane.activeEditorTabId === tabId
          pane.editorTabIds = pane.editorTabIds.filter((id) => id !== tabId)
          if (wasActive) {
            // Activate the ADJACENT tab so a close keeps you on a nearby tab
            // (VS Code-style): the tab that shifted into the closed slot (the
            // right neighbor), else the new last tab (the left neighbor when
            // the closed tab was last), else null when the pane holds none.
            //
            // Only a tab that still HAS content may be activated. A tabId whose
            // buffer is gone renders nothing. (When the slice is exercised
            // without a buffer list — pane-slice's own unit tests — every id
            // counts as alive: there is nothing to check against.)
            const known = state.buffers
            const isAlive = (id: string) => !Array.isArray(known) || known.some((b) => b.id === id)
            const alive = pane.editorTabIds.filter(isAlive)
            const rightNeighbor = pane.editorTabIds.slice(closedIndex).find(isAlive)
            pane.activeEditorTabId = rightNeighbor ?? alive[alive.length - 1] ?? null
          }
          if (pane.editorTabIds.length === 0) pane.editorOpen = false
          // Sync isUncloseable: the sole editor tab in a pane is uncloseable.
          syncSoleEditorTabCloseability(state, paneId)
          // Took the last thing this pane held — an empty pane is a fallback,
          // never a view, so it goes with it unless it is the last one left.
          dropEmptiedPanes(state)
        })
      },

      moveEditorTabToPane(tabId, fromPaneId, toPaneId) {
        set((state) => {
          const fromPane = state.panes[fromPaneId]
          const toPane = state.panes[toPaneId]
          if (!fromPane || !toPane) return
          fromPane.editorTabIds = fromPane.editorTabIds.filter((id) => id !== tabId)
          if (fromPane.activeEditorTabId === tabId) {
            fromPane.activeEditorTabId = fromPane.editorTabIds[0] ?? null
          }
          if (fromPane.editorTabIds.length === 0) fromPane.editorOpen = false

          if (!toPane.editorTabIds.includes(tabId)) toPane.editorTabIds.push(tabId)
          toPane.activeEditorTabId = tabId
          toPane.editorOpen = true

          state.activePaneId = toPaneId
          state.mostRecentActivePaneIds = [
            toPaneId,
            ...state.mostRecentActivePaneIds.filter((id) => id !== toPaneId),
          ]
          // Sync isUncloseable for both panes: the sole editor tab in each pane is uncloseable.
          syncSoleEditorTabCloseability(state, fromPaneId)
          syncSoleEditorTabCloseability(state, toPaneId)
        })
      },

      setEditorTabPreview(paneId, tabId) {
        set((state) => {
          const pane = state.panes[paneId]
          if (!pane || !pane.editorTabIds.includes(tabId)) return
          if (!Array.isArray(state.buffers)) return
          // Preview is a single-slot concept per pane: mark `tabId`'s content
          // as the preview and clear every other tab this pane holds.
          for (const id of pane.editorTabIds) {
            const buf = state.buffers.find((b) => b.id === id)
            if (buf) buf.isPreview = id === tabId
          }
        })
      },

      setEditorTabPinned(paneId, tabId, pinned) {
        set((state) => {
          const pane = state.panes[paneId]
          if (!pane || !pane.editorTabIds.includes(tabId)) return
          if (!Array.isArray(state.buffers)) return
          const buf = state.buffers.find((b) => b.id === tabId)
          if (buf) buf.isPinned = pinned
        })
      },

      setPaneLocked(paneId, locked) {
        set((state) => {
          const pane = state.panes[paneId]
          if (pane) pane.locked = locked
        })
      },

      reorderEditorTabs(paneId, tabId, targetIndex) {
        set((state) => {
          const pane = state.panes[paneId]
          if (!pane) return
          const ids = [...pane.editorTabIds]
          const startIndex = ids.indexOf(tabId)
          if (startIndex === -1) return
          const [moved] = ids.splice(startIndex, 1)
          const clampedTarget = Math.max(0, Math.min(targetIndex, ids.length))
          ids.splice(clampedTarget, 0, moved)
          pane.editorTabIds = ids
        })
      },

      resizePaneSplit(splitId, index, sizes) {
        set((state) => {
          if (findSplit(state.rootLayout, splitId)) {
            state.rootLayout = resizeFlattenedLayout(state.rootLayout, splitId, index, sizes)
          } else if (findSplit(state.bottomLayout, splitId)) {
            state.bottomLayout = resizeFlattenedLayout(state.bottomLayout, splitId, index, sizes)
          }
        })
      },

      distributePaneSplit(splitId) {
        set((state) => {
          if (findSplit(state.rootLayout, splitId)) {
            state.rootLayout = distributeSplit(state.rootLayout, splitId)
          } else {
            state.bottomLayout = distributeSplit(state.bottomLayout, splitId)
          }
        })
      },

      togglePaneFullscreen(paneId) {
        set((state) => {
          state.fullscreenPaneId = state.fullscreenPaneId === paneId ? null : paneId
        })
      },

      exitPaneFullscreen() {
        set((state) => {
          state.fullscreenPaneId = null
        })
      },

      getAllPaneGroups() {
        return Object.values(get().panes)
      },
      getPaneById(paneId) {
        return get().panes[paneId] ?? null
      },
      getPaneByEditorTabId(tabId) {
        return Object.values(get().panes).find((p) => p.editorTabIds.includes(tabId)) ?? null
      },
      getActivePane() {
        return get().panes[get().activePaneId] ?? null
      },

      clearEditorTabPreviewEverywhere() {
        set((state) => {
          if (!Array.isArray(state.buffers)) return
          for (const buf of state.buffers) buf.isPreview = false
        })
      },

      switchToNextEditorTab(paneId) {
        const state = get()
        const pane = state.panes[paneId]
        if (!pane || pane.editorTabIds.length <= 1) return
        const curr = pane.activeEditorTabId ? pane.editorTabIds.indexOf(pane.activeEditorTabId) : -1
        get().paneActions.activateEditorTabInPane(
          pane.id,
          pane.editorTabIds[(curr + 1) % pane.editorTabIds.length],
        )
      },

      switchToPreviousEditorTab(paneId) {
        const state = get()
        const pane = state.panes[paneId]
        if (!pane || pane.editorTabIds.length <= 1) return
        const curr = pane.activeEditorTabId ? pane.editorTabIds.indexOf(pane.activeEditorTabId) : 0
        get().paneActions.activateEditorTabInPane(
          pane.id,
          pane.editorTabIds[(curr - 1 + pane.editorTabIds.length) % pane.editorTabIds.length],
        )
      },

      navigateToPane(direction) {
        const state = get()
        for (const layout of [state.rootLayout, state.bottomLayout]) {
          const adj = getAdjacentLeafId(layout, state.activePaneId, direction)
          if (adj && state.panes[adj]) {
            set((s) => {
              s.activePaneId = adj
              s.mostRecentActivePaneIds = [
                adj,
                ...s.mostRecentActivePaneIds.filter((id) => id !== adj),
              ]
            })
            return
          }
        }
      },

      setPaneChat(paneId, chatId, runnerId) {
        set((state) => {
          const pane = state.panes[paneId]
          if (!pane) return
          const movedIn = chatId !== null && chatId !== pane.chatId
          const evicted = pane.chatId

          // Spec §8.4: "nothing you click ever costs you what you were
          // looking at." A pane genuinely swapping onto a DIFFERENT chat
          // (never an empty pane, never a same-chat/runner-only update, and
          // never a bare clear to null — none of those "cost" anything)
          // puts what it held into Recents whole first, same guards
          // `closePane`'s own archiving uses: skip a chat the daemon is
          // still working (its row lives on `agentChats.working` alone) and
          // skip one some other dormant entry already remembers. A fresh
          // id, not `paneId` — this pane is about to go LIVE on the new
          // chat, so reusing its id here would collide with the live entry
          // `deriveRecentsEntries` derives for it from the pane loop below.
          if (
            movedIn &&
            evicted &&
            !isChatWorking(evicted) &&
            !state.dormantArrangements.some((e) => e.chatIds.includes(evicted))
          ) {
            state.dormantArrangements.push({ id: nanoid(), chatIds: [evicted], state: 'dormant' })
          }

          pane.chatId = chatId
          pane.runnerId = runnerId
          // Spec §8.2: "whatever goes up leaves every arrangement that was
          // remembering it, and the arrangement you leave is remembered MINUS
          // whatever you took out of it... An arrangement left with nobody in
          // it goes." A chat moving fresh into a pane (never one already
          // showing there — that path never calls this at all, see
          // `performSidebarPaneDrop`'s "already up → reveal" branch) sheds
          // its membership in whatever MULTI-chat set still remembers it; the
          // survivors stay grouped under the same entry id.
          //
          // A SINGLE-chat entry is deliberately left alone here — it is not a
          // "set this chat is leaving", it IS this chat's own dormant/live
          // slot (spec §5.6: "restoring a dormant one — the row stays exactly
          // where it sits"). Stripping it here would delete that one record
          // outright, and `deriveRecentsEntries` would then re-derive the row
          // fresh from the pane loop — appended AFTER every remaining dormant
          // entry instead of staying put. Leaving it untouched means the SAME
          // record just recomputes to 'live' the next time Recents derives
          // (its `chatIds` still names this chat, and the chat is live again),
          // at its ORIGINAL slot — and `closePane`'s own "already remembered"
          // guard means the record is reused symmetrically on the way back
          // out, too.
          const strippable =
            movedIn &&
            state.dormantArrangements.some(
              (e) => e.chatIds.length > 1 && e.chatIds.includes(chatId),
            )
          if (strippable) {
            state.dormantArrangements = state.dormantArrangements
              .map((e) =>
                e.chatIds.length > 1 && e.chatIds.includes(chatId)
                  ? { ...e, chatIds: e.chatIds.filter((id) => id !== chatId) }
                  : e,
              )
              .filter((e) => e.chatIds.length > 0)
          }

          // An eviction (`use-workspace-agent-chats-stream.ts`'s `followRunner`
          // clearing a pane to null) leaves a pane holding nothing. That is a
          // fallback state, not a view: the pane collapses into its sibling
          // rather than standing in the layout as an empty box with a close
          // button on it. Only when it was genuinely emptied — a pane moving
          // ONTO a chat is the normal case and must not disturb the layout.
          if (chatId === null) dropEmptiedPanes(state)
        })
      },

      forgetDormantArrangement(entryId) {
        set((state) => {
          state.dormantArrangements = state.dormantArrangements.filter((e) => e.id !== entryId)
        })
      },

      forgetChat(chatId) {
        set((state) => {
          // No archiving on the way out, unlike `closePane`/`setPaneChat`: a
          // deleted chat has nothing left to come back to, and remembering it
          // is precisely the ghost row spec §9 says deletion must not leave.
          const clearedPaneIds: string[] = []
          for (const pane of Object.values(state.panes)) {
            if (pane.chatId !== chatId) continue
            pane.chatId = null
            pane.runnerId = null
            clearedPaneIds.push(pane.id)
          }
          if (state.dormantArrangements.some((e) => e.chatIds.includes(chatId))) {
            state.dormantArrangements = state.dormantArrangements
              .map((e) => ({ ...e, chatIds: e.chatIds.filter((id) => id !== chatId) }))
              .filter((e) => e.chatIds.length > 0)
          }

          // Spec §9: "If the last pane held something deleted it takes the
          // first chat still standing." Only reaches for a replacement when
          // the deletion left NOTHING else live anywhere in the window — in a
          // multi-pane layout the pane that lost its chat simply goes, the way
          // every other emptied pane does (`dropEmptiedPanes` at the tail,
          // which runs whichever branch this takes).
          const lastOneStanding =
            clearedPaneIds.length > 0 && !Object.values(state.panes).some((p) => p.chatId !== null)
          // "First" is the persisted Recents order (spec §5.6/§5.8) — the
          // only ordering this slice has anything to say about; a working-
          // but-viewless chat elsewhere would also qualify as "still
          // standing" but resolving one needs scanning every active
          // workspace store, which is Recents' own job
          // (`recents-for-project.ts`), not this window-level slice's.
          const standing = lastOneStanding
            ? state.dormantArrangements.find((e) => e.chatIds.length > 0)?.chatIds[0]
            : undefined
          const fallbackPane = standing ? state.panes[clearedPaneIds[0]] : undefined
          if (standing && fallbackPane) {
            fallbackPane.chatId = standing
            // The same survivor-stripping `closePane`/`setPaneChat` both do:
            // this chat is about to be LIVE again, in a pane, so it sheds
            // membership in whatever multi-chat entry still remembered it
            // (a single-chat entry is deliberately left alone — see
            // `setPaneChat`'s own note — so it just recomputes to 'live' at
            // its existing slot).
            state.dormantArrangements = state.dormantArrangements
              .map((e) =>
                e.chatIds.length > 1 && e.chatIds.includes(standing)
                  ? { ...e, chatIds: e.chatIds.filter((id) => id !== standing) }
                  : e,
              )
              .filter((e) => e.chatIds.length > 0)
          }

          // Whatever the deletion left chatless goes with it — the pane that
          // held the deleted chat is not a view any more, and spec §9's whole
          // point is that deletion "must not leave a name behind". The last
          // pane in the tree survives, chatless, as the fallback screen.
          if (clearedPaneIds.length > 0) dropEmptiedPanes(state)
        })
      },

      reorderRecentsEntry(entryId, targetId, mode, naturalOrder) {
        set((state) => {
          // Seed: any id THIS drag's project knows about that the ledger has
          // never tracked before gets appended, in the caller's own natural
          // order — never touching an id some OTHER project already placed.
          const known = new Set(state.recentsOrder)
          const seeded = state.recentsOrder.concat(naturalOrder.filter((id) => !known.has(id)))
          const withoutSource = seeded.filter((id) => id !== entryId)
          const targetIndex = withoutSource.indexOf(targetId)
          const insertAt =
            targetIndex === -1
              ? withoutSource.length
              : mode === 'after'
                ? targetIndex + 1
                : targetIndex
          withoutSource.splice(insertAt, 0, entryId)
          state.recentsOrder = withoutSource
        })
      },
    },
  }
}
