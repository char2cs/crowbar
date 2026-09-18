import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

/**
 * The ONE sibling sort every surface shares, identical to the daemon's
 * (api/internal/app/tree/node.go `compareNodes`): dense `order`, then row
 * kind, then creation time, then id. A level nobody has dragged is all ties
 * at 0, and a drop index is only meaningful when the rows are counted in the
 * same sequence they are drawn — on both sides.
 */
export const KIND_RANK = { folder: 0, branch: 1, chat: 2, workflow: 2, repo: 3 } as const

/** A repo's own header row ranks as a repo, never as the branch it draws as. */
function rankOf(row: Pick<SidebarRow, 'kind' | 'repoIcon'>): number {
  return row.repoIcon ? KIND_RANK.repo : KIND_RANK[row.kind]
}

export function compareSidebarRows(a: SidebarRow, b: SidebarRow): number {
  if (a.order !== b.order) return a.order - b.order
  const rank = rankOf(a) - rankOf(b)
  if (rank !== 0) return rank
  const createdA = a.createdAt ?? ''
  const createdB = b.createdAt ?? ''
  if (createdA !== createdB) return createdA < createdB ? -1 : 1
  return a.id < b.id ? -1 : a.id > b.id ? 1 : 0
}
