import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { compareSidebarRows } from './row-order'

export interface SidebarRowTree {
  roots: SidebarRow[]
  childrenByParent: Map<string, SidebarRow[]>
}

export function sidebarRowTree(rows: readonly SidebarRow[]): SidebarRowTree {
  const ids = new Set(rows.map((r) => r.id))
  const childrenByParent = new Map<string, SidebarRow[]>()
  const roots: SidebarRow[] = []
  for (const row of rows) {
    const parentId = row.parentId !== null && ids.has(row.parentId) ? row.parentId : null
    if (parentId === null) {
      roots.push(row)
      continue
    }
    const siblings = childrenByParent.get(parentId)
    if (siblings) siblings.push(row)
    else childrenByParent.set(parentId, [row])
  }
  roots.sort(compareSidebarRows)
  for (const siblings of childrenByParent.values()) siblings.sort(compareSidebarRows)
  return { roots, childrenByParent }
}
