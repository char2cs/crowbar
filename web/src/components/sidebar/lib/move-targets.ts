import { cycleSafeAllowedModes } from '@/components/sidebar/lib/sidebar-drag-hover'
import { planRowDrop, UNSUPPORTED } from '@/components/sidebar/lib/drop-plan'
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

/** The folders and repo headers `row` can be moved into: exactly what dragging it there would
 *  drop it into, in tree order, so the menu is the drag's keyboard twin. Its own parent is left
 *  out, since it is already there. */
export function moveTargets(row: SidebarRow, rows: readonly SidebarRow[]): SidebarRow[] {
  const byId = new Map(rows.map((r) => [r.id, r]))
  return rows.filter(
    (target) =>
      (target.kind === 'folder' || target.repoIcon !== undefined) &&
      target.id !== row.id &&
      target.id !== row.parentId &&
      cycleSafeAllowedModes([row], { ...target, path: pathOf(target, byId) }).into &&
      movesSomething(planRowDrop([row], target, 'into')),
  )
}

// An empty plan is a drop the tree takes as a no-op (a thread onto a branch): no destination.
function movesSomething(plan: ReturnType<typeof planRowDrop>): boolean {
  return plan !== UNSUPPORTED && plan.length > 0
}
