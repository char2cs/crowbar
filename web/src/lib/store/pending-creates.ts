import { create } from 'zustand'

/**
 * A create in flight, drawn as a real row at the exact slot the finished
 * create will land in — spec: "the placement of the pending row is exactly
 * where it lands when finished," never appended loosely at the end of
 * whatever the tree happens to show.
 *
 * A fork ('branch') asks for its name first — `status` starts 'naming' and a
 * real row-shaped input replaces the row until confirmed. A thread ('chat')
 * has nothing to name — created rows go straight to 'creating'. Both then
 * show a spinner until the real row is OBSERVED to exist (never merely once
 * the create's promise resolves — the two are not the same instant), or an
 * inline error with a dismiss if it fails.
 */
export interface PendingCreateEntry {
  tempId: string
  kind: 'branch' | 'chat'
  /** Which project's panel draws this row — `rowsForProject`'s own filter,
   *  mirrored here so a pending row appears in exactly the one panel the
   *  real row will land in and nowhere else. */
  projectId: string
  /** The SidebarRow.parentId space this row will land in once real. */
  parentId: string
  /** The sibling order it will land at — computed once, at click time, from
   *  the SAME sibling data the tree already shows, so it never needs to be
   *  recomputed once creation actually lands. */
  order: number
  workspaceId: string | null
  ownsWorktree: boolean
  status: 'naming' | 'creating' | 'error'
  /** The typed branch name once confirmed; '' while naming or for a thread,
   *  which has no name of its own. */
  label: string
  error?: string
  /**
   * The REAL row's id, attached the moment the create's own request resolves
   * — before its placement is confirmed correct, not after.
   *
   * A create's mint (the `Chat`/`Workspace` aggregate) and its placement (a
   * separate `Node` write, `CreateChat`/`placeChat`, chats.go) are two
   * sequential backend writes, not one — so the real row can exist in the
   * client's own store for a beat with its OLD/default placement (root)
   * before the correction lands. This id is how a caller tells a row-list
   * builder "hide the real row at this id until you see me clear" (see
   * `space-scroller.tsx`'s `unconfirmedRealIds` filter) — the correctly
   * PLACED pending row above stays the only visible stand-in the whole time,
   * so there is nothing to visually correct once this entry finally clears:
   * `waitForHomeChat`/`chatHasLanded`/`forkHasLanded` (space-content-
   * actions.ts) already gate that clear on the real row's placement actually
   * matching, this just keeps that real row invisible in the meantime rather
   * than rendering it wrong first and fixing it a moment later.
   */
  realId?: string
}

interface PendingCreatesState {
  entries: PendingCreateEntry[]
  /** Arms the naming input for a fork create. Only one is ever open at once
   *  (matching the old tree's own single `creatingChildOf`) — but THIS store
   *  does not enforce that itself: the caller (`space-content-actions.ts`'s
   *  `handleCreate`) cancels any other naming entry first, via
   *  `cancelPendingCreate`, which also releases the `createInFlight` lock and
   *  `armedBranchCreates` entry that entry's OWN close would otherwise leak —
   *  state this store knows nothing about. */
  startNaming: (entry: Omit<PendingCreateEntry, 'status' | 'label' | 'error'>) => void
  /** Confirms a naming entry into 'creating' with the typed label. */
  confirmNaming: (tempId: string, label: string) => void
  /** Adds a thread create straight into 'creating' — no naming step. */
  addCreating: (entry: Omit<PendingCreateEntry, 'status' | 'label' | 'error'>) => void
  /** Attaches the real row's id once the create's own request resolves — see
   *  `PendingCreateEntry.realId`'s own doc. A no-op if the entry already left
   *  (cleared or errored while the request was still in flight). */
  attachRealId: (tempId: string, realId: string) => void
  setError: (tempId: string, error: string) => void
  /** Drops an entry outright — a naming input the user cancelled (never
   *  reached the network, nothing to roll back), or an error the user
   *  dismissed. */
  clear: (tempId: string) => void
}

export function getInitialPendingCreatesState() {
  return { entries: [] as PendingCreateEntry[] }
}

export const usePendingCreatesStore = create<PendingCreatesState>()((set) => ({
  ...getInitialPendingCreatesState(),
  startNaming: (entry) =>
    set((s) => ({ entries: [...s.entries, { ...entry, status: 'naming', label: '' }] })),
  confirmNaming: (tempId, label) =>
    set((s) => ({
      entries: s.entries.map((e) => (e.tempId === tempId ? { ...e, status: 'creating', label } : e)),
    })),
  addCreating: (entry) =>
    set((s) => ({ entries: [...s.entries, { ...entry, status: 'creating', label: '' }] })),
  attachRealId: (tempId, realId) =>
    set((s) => ({
      entries: s.entries.map((e) => (e.tempId === tempId ? { ...e, realId } : e)),
    })),
  setError: (tempId, error) =>
    set((s) => ({
      entries: s.entries.map((e) => (e.tempId === tempId ? { ...e, status: 'error', error } : e)),
    })),
  clear: (tempId) => set((s) => ({ entries: s.entries.filter((e) => e.tempId !== tempId) })),
}))
