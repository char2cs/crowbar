import { NO_MODES } from '@/components/tree-dnd/drop-core'
import { SIDEBAR_DROP_POLICY } from '@/components/sidebar/lib/sidebar-drop-policy'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import type { NativeRowDrop } from '@/lib/native-chrome'

/** A target as the desktop drag reads it off the DOM: folded or open, with or without children. */
export function dropTargetRow(
  rows: readonly SidebarRow[],
  collapsed: ReadonlySet<string>,
  id: string,
) {
  const row = rows.find((r) => r.id === id)
  if (!row) return null
  const hasChildren = rows.some((r) => r.parentId === id)
  return { ...row, hasChildren, expanded: hasChildren && !collapsed.has(id) }
}

/**
 * Where the desktop's drop rules let `subjectId` land, for every row: its policy, plus the
 * desktop hit test's guard against dropping a row into its own subtree.
 */
export function rowDropTargets(
  rows: readonly SidebarRow[],
  collapsed: ReadonlySet<string>,
  subjectId: string,
): NativeRowDrop | null {
  const subject = rows.find((r) => r.id === subjectId)
  if (!subject) return null
  const parentOf = new Map(rows.map((r) => [r.id, r.parentId]))
  const insideSubject = (id: string) => {
    const seen = new Set<string>()
    for (
      let at: string | null | undefined = parentOf.get(id);
      at && !seen.has(at);
      at = parentOf.get(at)
    ) {
      if (at === subjectId) return true
      seen.add(at)
    }
    return false
  }
  const targets: NativeRowDrop['targets'] = {}
  for (const row of rows) {
    const target = dropTargetRow(rows, collapsed, row.id)!
    targets[row.id] = {
      modes: insideSubject(row.id) ? NO_MODES : SIDEBAR_DROP_POLICY.allowedModes([subject], target),
      band: SIDEBAR_DROP_POLICY.edgeBandFor(row.kind),
    }
  }
  return { subjectId, targets }
}
