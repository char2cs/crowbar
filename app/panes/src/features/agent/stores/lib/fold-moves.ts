import type { LedgerRow } from '@/features/agent/api/chat-rows'
import type { FeedFold } from '@/features/agent/stores/lib/fold-rows'
import { moveItemId, type MarkerItem } from '@/features/agent/stores/lib/transcript-items'

export function applyMove(fold: FeedFold, row: LedgerRow<'move'>) {
  if (row.scope) return
  const id = moveItemId(row.body.requestId)
  const asked = fold.item<MarkerItem>(id)?.move
  fold.put({
    kind: 'marker',
    id,
    seq: row.seq,
    turnId: row.turnId,
    at: row.at,
    marker: 'move',
    move: { ...asked, ...row.body },
  })
  fold.unplace(id)
  fold.place(id, '')
}
