import { useSidebarStore } from '@/lib/store/sidebar'
import { useSidebarSelectionStore } from '@/lib/store/sidebar-selection'
import { readDropRow, type DropRow } from './drop-target-dom'
import { foldAway, releaseOnExpand, snapshotOnCollapse, type TreeIndex } from './keep-set'

/**
 * The two-state keep model, wired to the DOM and the stores.
 *
 * `keep-set.ts` holds the rules as pure functions; this is the only place that
 * decides WHEN to apply them. Everything here runs from an event handler, never
 * from a render.
 */

/**
 * Every row on screen, top to bottom as they are drawn.
 *
 * Read off the tree rather than derived from the node graph: what a user means
 * by "the rows between these two" is what they can see, and a collapsed subtree
 * contributes none of them. One query, so the shift-range, the drag and the
 * keyboard cannot end up walking the tree in three slightly different orders.
 */
export function visibleRows(root: ParentNode = document): DropRow[] {
  const out: DropRow[] = []
  for (const el of root.querySelectorAll('[role="treeitem"]')) {
    const row = readDropRow(el)
    if (row) out.push(row)
  }
  return out
}

/**
 * The rows a shift-range can span.
 *
 * The same list, minus the repo and project rows: the multiselection is a
 * workspace and folder idea, and the two coarser rows are their own
 * destinations. Keyboard navigation reads {@link visibleRows} instead, because
 * a row you cannot arrow onto is a row you cannot reach at all.
 */
export function visibleRowIds(root: ParentNode = document): string[] {
  const out: string[] = []
  for (const row of visibleRows(root)) {
    if (row.kind === 'workspace' || row.kind === 'folder') out.push(row.id)
  }
  return out
}

/** The modifier keys a row click carries. */
interface SelectionModifiers {
  metaKey: boolean
  ctrlKey: boolean
  shiftKey: boolean
}

/**
 * Apply what a click on `id` means for the multiselection.
 *
 * Returns true when the click WAS the selection gesture and must go no further:
 * cmd-clicking a row is not a request to open it.
 *
 * A plain click clears the multiselection. That clearing, not the styling, is
 * what stops several lit rows accumulating — and it deliberately leaves the keep
 * set alone, so a row you kept through a collapse cannot evaporate because you
 * clicked somewhere else.
 */
export function handleRowSelectionClick(
  e: SelectionModifiers,
  id: string,
  index?: TreeIndex,
): boolean {
  const selection = useSidebarSelectionStore.getState()
  if (e.metaKey || e.ctrlKey) {
    const wasSelected = selection.selected.has(id)
    selection.toggleSelected(id)
    // Turning a row off is also how ONE kept row is dismissed — the parent's
    // fold-away control is what dismisses the whole set. The subtree goes with
    // it, because that is what the row was holding on screen.
    if (wasSelected && selection.kept.has(id)) {
      selection.releaseRows(index ? [id, ...index.descendantsOf(id)] : [id])
    }
    return true
  }
  if (e.shiftKey) {
    selection.selectRange(id, visibleRowIds())
    return true
  }
  selection.clearSelected(id)
  return false
}

/**
 * Collapse or expand `rowId`, taking or releasing its keep-set snapshot.
 *
 * On the way closed the rows that were multiselected or open under this parent
 * are promoted into the keep set; on the way open they are released, because
 * they are visible again on their own account.
 */
export function toggleRowKeepingRows(
  rowId: string,
  index: TreeIndex,
  activeWorkspaceId: string,
): void {
  const wasCollapsed = useSidebarStore.getState().collapsedWorkspaces.has(rowId)
  applyKeepSetToggle(rowId, index, activeWorkspaceId, wasCollapsed)
  useSidebarStore.getState().toggleWorkspace(rowId)
}

/** The same, for a repo header row, which owns its own collapse set. */
export function toggleRepoKeepingRows(
  repoId: string,
  index: TreeIndex,
  activeWorkspaceId: string,
): void {
  const wasCollapsed = useSidebarStore.getState().collapsedRepos.has(repoId)
  applyKeepSetToggle(repoId, index, activeWorkspaceId, wasCollapsed)
  useSidebarStore.getState().toggleRepo(repoId)
}

function applyKeepSetToggle(
  rowId: string,
  index: TreeIndex,
  activeWorkspaceId: string,
  wasCollapsed: boolean,
): void {
  const selection = useSidebarSelectionStore.getState()
  if (wasCollapsed) selection.releaseRows(releaseOnExpand(rowId, index))
  else {
    selection.keepRows(
      snapshotOnCollapse(rowId, index, selection.selected, activeWorkspaceId || null),
    )
  }
}

/**
 * The parent's fold-away control: let go of every row it is holding, without
 * opening it.
 *
 * This is the only thing that dismisses a keep set wholesale — cmd-clicking a
 * row off removes it one at a time.
 */
export function foldAwayRows(rowId: string, index: TreeIndex): void {
  const selection = useSidebarSelectionStore.getState()
  selection.setKept(foldAway(rowId, index, selection.kept))
}
