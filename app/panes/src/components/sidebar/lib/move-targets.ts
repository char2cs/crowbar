import { cycleSafeAllowedModes } from '@/components/sidebar/lib/sidebar-drag-hover'
import { planRowDrop } from '@/components/sidebar/lib/drop-plan'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

/** A row's ancestor chain as the tree publishes it for a drag (`/root/…/row/`), from parentIds. */
function pathOf(row: SidebarRow, byId: ReadonlyMap<string, SidebarRow>): string {
  let path = `${row.id}/`
  const seen = new Set([row.id])
  for (let up = row.parentId; up && !seen.has(up); up = byId.get(up)?.parentId ?? null) {
    seen.add(up)
    path = `${up}/${path}`
  }
  return `/${path}`
}

/** The folders and repo headers `row` can be moved into, and for a chat every branch too: exactly
 *  what dragging it there would drop it into, in tree order, so the menu is the drag's keyboard
 *  twin. Its own parent is left out, since it is already there. */
export function moveTargets(row: SidebarRow, rows: readonly SidebarRow[]): SidebarRow[] {
  const byId = new Map(rows.map((r) => [r.id, r]))
  return rows.filter(
    (target) =>
      (target.kind === 'folder' ||
        target.repoIcon !== undefined ||
        (row.kind === 'chat' && target.kind === 'branch')) &&
      target.id !== row.id &&
      target.id !== row.parentId &&
      cycleSafeAllowedModes([row], { ...target, path: pathOf(target, byId) }).into &&
      // An empty plan is a drop the tree takes as a no-op: no destination.
      planRowDrop([row], target, 'into').length > 0,
  )
}
