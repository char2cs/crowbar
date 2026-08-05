import { create } from 'zustand'

/**
 * The removal tray: rows that are on their way out but have not gone yet.
 *
 * This is deliberately NOT the delete path. Nothing here touches the daemon and
 * nothing here mutates the sidebar tree — a held row is simply hidden, and
 * cancelling puts it back exactly as it was, its subtree and (for a repo) its
 * contents included. Only when a hold is committed does anything destructive
 * happen, and that lives in `components/layout/removal-commit.ts` next to the
 * navigation it has to do.
 *
 * `hiddenIds` is kept beside `entries` rather than derived from it, because the
 * two do not end together: a committed entry leaves the tray immediately while
 * its rows stay hidden until the daemon's tombstones actually arrive. Deriving
 * one from the other would flash every removed row back onto the screen for the
 * length of a round trip.
 */

/** How long a held row waits before its removal fires. */
export const REMOVAL_DRAIN_MS = 8000

/** Stable empties, so a tree with nothing held hands out one identity. */
const NO_ENTRIES: readonly RemovalEntry[] = []
const NO_IDS: ReadonlySet<string> = new Set<string>()

/**
 * One row on its way out, worked out before anything is hidden.
 *
 * The shape lives here rather than beside the planner that fills it in, so the
 * store depends on nothing in `components/`.
 */
export interface RemovalDraft {
  kind: 'workspace' | 'folder' | 'repo' | 'project'
  /** The row itself: a workspace, folder, repo or project id. */
  id: string
  /** What the row reads as, for the tray row and the pane's overlay. */
  label: string
  projectId: string
  /** The owning repo; for a repo removal this is the repo itself, and for a
   *  project removal there is no single one, so it is ''. */
  repoId: string
  /** Every row hidden while this waits — the row and whatever it takes with it. */
  hiddenIds: readonly string[]
  /** How many rows go with it beyond itself; 0 draws no count. */
  extra: number
  /**
   * Where to go if the workspace being viewed is inside what is going, resolved
   * BEFORE anything is hidden — afterwards the tree no longer has the answer.
   */
  fallbackWsId: string | null
}

export interface RemovalEntry extends RemovalDraft {
  /** Identity of the tray row, distinct from the row's own id. */
  entryId: string
  /**
   * When the removal fires, or null when it waits on an answer instead.
   *
   * A repo takes every worktree under it, and a project takes every repo — so
   * neither runs a clock that could run out while you are looking somewhere
   * else. Both ask instead, and both then ask AGAIN in a modal that spells out
   * the cascade, because eight seconds of undo is not a proportionate safety net
   * for deleting a whole project's worth of work.
   */
  deadlineAt: number | null
}

interface RemovalTrayState {
  /** The tray's rows, oldest first. */
  entries: readonly RemovalEntry[]
  /** Rows the sidebar must not draw while their removal is pending. */
  hiddenIds: ReadonlySet<string>
  /** Hold rows: hide them and put a row in the tray for each. */
  hold: (drafts: readonly RemovalDraft[]) => void
  /** The user kept it: drop the tray row and show its rows again. */
  cancel: (entryId: string) => void
  /** The removal has been sent: drop the tray row, keep its rows hidden. */
  settle: (entryId: string) => void
  /** Stop hiding these rows — the daemon confirmed, or refused. */
  release: (ids: readonly string[]) => void
}

export function getInitialRemovalState() {
  return { entries: NO_ENTRIES, hiddenIds: NO_IDS }
}

export const useRemovalTrayStore = create<RemovalTrayState>()((set) => ({
  ...getInitialRemovalState(),

  hold: (drafts) =>
    set((s) => {
      if (drafts.length === 0) return s
      const now = Date.now()
      const entries = drafts.map((draft) => ({
        ...draft,
        entryId: crypto.randomUUID(),
        // A repo or a project waits on Cancel / Remove; everything else drains.
        deadlineAt:
          draft.kind === 'repo' || draft.kind === 'project' ? null : now + REMOVAL_DRAIN_MS,
      }))
      const hiddenIds = new Set(s.hiddenIds)
      for (const entry of entries) for (const id of entry.hiddenIds) hiddenIds.add(id)
      return { entries: [...s.entries, ...entries], hiddenIds }
    }),

  cancel: (entryId) =>
    set((s) => {
      const entry = s.entries.find((e) => e.entryId === entryId)
      if (!entry) return s
      const hiddenIds = new Set(s.hiddenIds)
      for (const id of entry.hiddenIds) hiddenIds.delete(id)
      return { entries: s.entries.filter((e) => e !== entry), hiddenIds }
    }),

  settle: (entryId) =>
    set((s) => {
      const entries = s.entries.filter((e) => e.entryId !== entryId)
      return entries.length === s.entries.length ? s : { entries }
    }),

  release: (ids) =>
    set((s) => {
      if (ids.length === 0 || s.hiddenIds.size === 0) return s
      const hiddenIds = new Set(s.hiddenIds)
      let removed = false
      for (const id of ids) removed = hiddenIds.delete(id) || removed
      return removed ? { hiddenIds } : s
    }),
}))
