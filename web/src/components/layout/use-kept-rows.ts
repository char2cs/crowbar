import { useMemo } from 'react'
import { useSidebarSelectionStore } from '@/lib/store/sidebar-selection'
import { isHoldingRows, keptRootsUnder } from './keep-set'
import type { SidebarRepoTree } from './workspace-tree-utils'

/** Stable empties, so a row that holds nothing hands out one identity. */
const NO_KEPT: ReadonlySet<string> = new Set<string>()
const NO_ROWS: readonly string[] = []

export interface KeptRows {
  /** Folded, but still showing rows — the state the PARENT wears. */
  holding: boolean
  /** The outermost kept rows, to draw one indent step under this row. */
  roots: readonly string[]
}

/**
 * What a row is holding on screen while it is folded.
 *
 * The selector is gated on the row being folded and having something to fold:
 * an expanded row and a leaf both read the stable empty set, so the keep set
 * changing re-renders only the handful of rows that could possibly be holding
 * anything. A row's multiselected-ness is a separate, narrower subscription —
 * the two states are kept apart in the store for exactly this reason.
 */
export function useKeptRows(
  rowId: string,
  tree: SidebarRepoTree,
  collapsed: boolean,
  hasChildren: boolean,
): KeptRows {
  const kept = useSidebarSelectionStore((s) => (collapsed && hasChildren ? s.kept : NO_KEPT))

  return useMemo(() => {
    if (kept.size === 0 || !isHoldingRows(rowId, tree.index, kept)) {
      return { holding: false, roots: NO_ROWS }
    }
    return { holding: true, roots: keptRootsUnder(rowId, tree.index, kept) }
  }, [kept, rowId, tree])
}
