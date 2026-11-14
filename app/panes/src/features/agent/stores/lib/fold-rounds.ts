import type { LedgerRow } from '@/features/agent/api/chat-rows'
import type { FeedFold } from '@/features/agent/stores/lib/fold-rows'

/** A turn's main-scope calls since its last message. One response's pres all
 *  come before any of their ends, so a pre after an end opens the next round.
 *  `closing` is a message read above a round whose calls had all ended. */
export interface CallRound {
  first?: string
  open: string[]
  ended: boolean
  closing?: string
}

export function noteCall(fold: FeedFold, row: LedgerRow<'tool_call'>, itemId: string) {
  const { turnId: turn, body } = row
  if (row.scope || !turn) return
  if (body.phase !== 'pre') return endCall(fold, turn, body.toolId)
  const round = fold.facts.calls[turn]
  const fresh = !round?.first || round.ended
  const open = [...(fresh ? [] : round.open), body.toolId]
  fold.record('calls')[turn] = { first: fresh ? itemId : round.first, open, ended: false }
}

export function endCall(fold: FeedFold, turn: string, toolId: string) {
  const round = fold.facts.calls[turn]
  if (!round?.first) return
  const open = round.open.filter((id) => id !== toolId)
  fold.record('calls')[turn] = { ...round, open, ended: true }
}

/** A model says nothing while its calls run, so text landing then was said
 *  before them. claude also reports a one-chunk message only when its response
 *  ended, maybe after all its calls ended: then it may instead be the closing
 *  reply, which the turn's end tells. */
export function placeMessage(fold: FeedFold, row: LedgerRow<'assistant_text'>, id: string) {
  const turn = row.turnId ?? ''
  const round = row.scope ? undefined : fold.facts.calls[turn]
  const whole = row.body.index === 0 && row.body.final === true
  const said = whole || !!round?.open.length
  const above = fold.place(id, row.scope ?? '', said ? round?.first : undefined)
  if (!round) return
  const closing = above && round.open.length === 0 ? id : undefined
  fold.record('calls')[turn] = { open: [], ended: false, closing }
}

export function settleRound(fold: FeedFold, turn: string) {
  const closing = fold.facts.calls[turn]?.closing
  if (fold.facts.calls[turn]) delete fold.record('calls')[turn]
  if (!closing) return
  fold.unplace(closing)
  fold.place(closing, '')
}
