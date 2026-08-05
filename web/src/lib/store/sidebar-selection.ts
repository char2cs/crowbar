import { create } from 'zustand'

/**
 * The sidebar's two selection-shaped states — which are two states, not one.
 *
 * `selected` is the multiselection: transient, built by cmd/shift-click, drawn
 * exactly like the open workspace, and wiped by the next plain click. That
 * wiping — not the styling — is what stops a crowd of lit rows accumulating.
 *
 * `kept` is the keep set: a snapshot taken at the moment a parent collapses, of
 * whatever was multiselected or open *at that moment*. It carries no styling of
 * its own. Nothing here writes both fields in one call, which is the mechanical
 * guarantee that clearing the selection cannot disturb a row you deliberately
 * kept: zustand merges the partial, so the untouched field keeps its identity.
 *
 * The rules over these sets live in `components/layout/keep-set.ts` as pure
 * functions; this store only holds the result. Keeping them apart is what lets
 * a store hold UI state without importing from `components/`.
 */

/** Stable empties, so a store that has never been written hands out one identity. */
const NO_IDS: ReadonlySet<string> = new Set<string>()

interface SidebarSelectionState {
  /** The multiselection. Mirrored onto each row's `aria-selected`. */
  selected: ReadonlySet<string>
  /** The keep set: rows still on screen under a parent that has folded away. */
  kept: ReadonlySet<string>
  /** Where a shift-range measures from — the last row touched deliberately. */
  anchorId: string | null
  /** cmd/ctrl-click: add or remove one row, and re-anchor on it. */
  toggleSelected: (id: string) => void
  /**
   * shift-click: select everything between the anchor and `id`.
   *
   * `order` is the rows as they are drawn, top to bottom — the range a user
   * means is the visual one, so it cannot be derived from the tree.
   */
  selectRange: (id: string, order: readonly string[]) => void
  /** A plain click: drop the multiselection and anchor here instead. */
  clearSelected: (anchorId?: string | null) => void
  /** Add rows to the keep set (a parent collapsing over them). */
  keepRows: (ids: readonly string[]) => void
  /** Drop rows from the keep set (a parent expanding again). */
  releaseRows: (ids: readonly string[]) => void
  /** Replace the keep set outright — the fold-away control's result. */
  setKept: (kept: ReadonlySet<string>) => void
}

export function getInitialSelectionState() {
  return { selected: NO_IDS, kept: NO_IDS, anchorId: null }
}

export const useSidebarSelectionStore = create<SidebarSelectionState>()((set) => ({
  ...getInitialSelectionState(),

  toggleSelected: (id) =>
    set((s) => {
      const next = new Set(s.selected)
      if (!next.delete(id)) next.add(id)
      return { selected: next, anchorId: id }
    }),

  selectRange: (id, order) =>
    set((s) => {
      const from = s.anchorId === null ? -1 : order.indexOf(s.anchorId)
      const to = order.indexOf(id)
      // No usable anchor (nothing clicked yet, or it has since left the tree):
      // the shift-click degrades to selecting the one row it landed on.
      if (from === -1 || to === -1) return { selected: new Set(s.selected).add(id), anchorId: id }
      const [lo, hi] = from < to ? [from, to] : [to, from]
      const next = new Set(s.selected)
      for (const rowId of order.slice(lo, hi + 1)) next.add(rowId)
      return { selected: next }
    }),

  clearSelected: (anchorId = null) =>
    set((s) =>
      // Already empty and already anchored here: hand back the same sets rather
      // than a fresh identity, so a plain click on an unselected tree is free.
      s.selected.size === 0 && s.anchorId === anchorId ? s : { selected: NO_IDS, anchorId },
    ),

  keepRows: (ids) =>
    set((s) => {
      if (ids.length === 0) return s
      const next = new Set(s.kept)
      for (const id of ids) next.add(id)
      return { kept: next }
    }),

  releaseRows: (ids) =>
    set((s) => {
      if (ids.length === 0 || s.kept.size === 0) return s
      const next = new Set(s.kept)
      let removed = false
      for (const id of ids) removed = next.delete(id) || removed
      return removed ? { kept: next } : s
    }),

  setKept: (kept) => set({ kept }),
}))
