import { anyModeAllowed, type DropMode } from '@/components/tree-dnd/drop-core'
import {
  cycleSafeAllowedModes,
  hitTest,
  rowDom,
  type SidebarPaneZone,
} from '@/components/sidebar/lib/sidebar-drag-hover'
import { toast } from '@/features/window/stores/toast-store'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

export interface DropHandlers {
  subjectsFor: (rowId: string) => SidebarRow[]
  onDrop: (
    subjects: SidebarRow[],
    target: SidebarRow,
    mode: DropMode,
    targetInRecents?: boolean,
  ) => void
  onPaneDrop: (subjects: SidebarRow[], paneId: string, zone: SidebarPaneZone) => void
}

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
