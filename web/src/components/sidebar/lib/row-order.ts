import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { createdInstant } from '@/lib/store/created-instant'

// The one sibling sort every surface shares, matching the daemon's compareNodes: order, then
// kind, creation time, id. An undragged level is all ties at 0, and a drop index only means
// something if both sides count rows in the order they are drawn.
const KIND_RANK = { folder: 0, branch: 1, chat: 2, workflow: 2, repo: 3 } as const

type TieKeys = Pick<SidebarRow, 'id' | 'kind' | 'repoIcon'>

/** A repo's own header row ranks as a repo, never as the branch it draws as. */
function rankOf(row: Pick<TieKeys, 'kind' | 'repoIcon'>): number {
  return row.repoIcon ? KIND_RANK.repo : KIND_RANK[row.kind]
}

/** The daemon ties on the Node id, which for a repo header is the REPO id —
 *  never the owning-chat id the row is drawn by. */
function tieIdOf(row: Pick<TieKeys, 'id' | 'repoIcon'>): string {
  return row.repoIcon?.repoId ?? row.id
}

export function compareSidebarRows(a: SidebarRow, b: SidebarRow): number {
  if (a.order !== b.order) return a.order - b.order
  const rank = rankOf(a) - rankOf(b)
  if (rank !== 0) return rank
  const created = createdInstant(a.createdAt) - createdInstant(b.createdAt)
  if (created !== 0) return created
  const idA = tieIdOf(a)
  const idB = tieIdOf(b)
  return idA < idB ? -1 : idA > idB ? 1 : 0
}
