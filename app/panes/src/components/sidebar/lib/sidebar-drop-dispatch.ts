import { anyModeAllowed, type DropMode } from '@/components/tree-dnd/drop-core'
import {
  cycleSafeAllowedModes,
  hitTest,
  rowDom,
  type SidebarPaneZone,
} from '@/components/sidebar/lib/sidebar-drag-hover'
import { toast } from '@/features/window/stores/toast-store'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

/** Where a released drag commits. */
export interface DropHandlers {
  /** The rows a drag starting on `rowId` carries, in tree order (the caller's selection);
   *  a release over a row also resolves its live target through it. */
  subjectsFor: (rowId: string) => SidebarRow[]
  /** `targetInRecents`: the row released over is a Recents tab (`view-row.tsx`), where spec §8.1
   *  gives the same geometry another meaning than over a tree row. */
  onDrop: (
    subjects: SidebarRow[],
    target: SidebarRow,
    mode: DropMode,
    targetInRecents?: boolean,
  ) => void
  /** Every pane drop adds a view (spec §8.1); none removes one. */
  onPaneDrop: (subjects: SidebarRow[], paneId: string, zone: SidebarPaneZone) => void
}

/** Commits `subjects` released at (`x`, `y`): onto a pane, onto a row, or, when the row
 *  under the pointer refused every mode, a toast saying so. */
export function dispatchDrop(
  subjects: readonly SidebarRow[],
  x: number,
  y: number,
  handlers: DropHandlers,
): void {
  const hit = hitTest(x, y, subjects)
  if (hit?.kind === 'pane') {
    handlers.onPaneDrop([...subjects], hit.paneId, hit.zone)
    return
  }
  if (hit?.kind === 'row') {
    commitRowDrop(subjects, hit.row.id, hit.row.mode, Boolean(hit.row.inRecents), handlers)
    return
  }
  toastRefusal(subjects, x, y)
}

/** Commits a drop onto row `targetId`. The id is what the DOM published, not a SidebarRow: the
 *  caller gets the live one, and nothing when it left the tree since the hit test. */
export function commitRowDrop(
  subjects: readonly SidebarRow[],
  targetId: string,
  mode: DropMode,
  inRecents: boolean,
  handlers: DropHandlers,
): void {
  const target = handlers.subjectsFor(targetId).find((r) => r.id === targetId)
  if (target) handlers.onDrop([...subjects], target, mode, inRecents)
}

// A null hit is also a row whose policy refused every mode, which would otherwise look
// like a drop that silently did not stick. A release back on a dragged row is a cancel.
function toastRefusal(subjects: readonly SidebarRow[], x: number, y: number): void {
  for (const el of document.elementsFromPoint(x, y)) {
    const row = rowDom.read(el)
    if (!row) continue
    const droppedOnItself = subjects.some((s) => s.id === row.id)
    if (!droppedOnItself && !anyModeAllowed(cycleSafeAllowedModes(subjects, row))) {
      toast.error(`Can't move ${subjects[0]?.label || 'this row'} there`)
    }
    break
  }
}
