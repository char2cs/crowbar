import type { LedgerRow } from '@/features/agent/api/chat-rows'
import type { FeedFold } from '@/features/agent/stores/lib/fold-rows'

/** One provider response's main-scope calls: its pres all come before any of
 *  their ends, so a call begun after an end opens the next response's round. */
export interface CallRound {
  /** The item of the round's first call. */
  first: string
  /** Its calls not yet ended. */
  open: string[]
  ended: boolean
}

/** Something the agent did after a message placed above an ended round shows
 *  the message was said before that round, not after it. */
function goneOn(fold: FeedFold, turn: string) {
  if (fold.facts.closing[turn]) delete fold.record('closing')[turn]
}

/** Notes a main-scope call's start or end in its turn's round. */
export function noteCall(fold: FeedFold, row: LedgerRow<'tool_call'>, itemId: string) {
  const turn = row.turnId
  if (row.scope || !turn) return
  const { toolId, phase } = row.body
  const round = fold.facts.calls[turn]
  if (phase !== 'pre') {
    endCall(fold, turn, toolId)
    return
  }
  goneOn(fold, turn)
  fold.record('calls')[turn] =
    round && !round.ended
      ? { ...round, open: [...round.open, toolId] }
      : { first: itemId, open: [toolId], ended: false }
}

export function endCall(fold: FeedFold, turn: string, toolId: string) {
  const round = fold.facts.calls[turn]
  if (!round) return
  fold.record('calls')[turn] = {
    ...round,
    open: round.open.filter((id) => id !== toolId),
    ended: true,
  }
}

/** Places a new main-scope message. One reported whole, in a single final
 *  chunk, was reported when its response ended: it reads above that
 *  response's calls. If they had all ended, it may instead be the closing
 *  reply, which the turn's end tells (`settleRound`). */
export function placeMessage(fold: FeedFold, row: LedgerRow<'assistant_text'>, id: string) {
  const turn = row.turnId ?? ''
  goneOn(fold, turn)
  const round = fold.facts.calls[turn]
  if (round) delete fold.record('calls')[turn]
  const whole = row.body.index === 0 && row.body.final === true
  if (!round || !whole || !fold.placed(round.first)) {
    fold.place(id, '')
    return
  }
  fold.place(id, '', round.first)
  if (round.open.length === 0) fold.record('closing')[turn] = id
}

/** A turn that ended on a message read above its ended calls closed on it. */
export function settleRound(fold: FeedFold, turn: string) {
  const closing = fold.facts.closing[turn]
  if (closing) {
    delete fold.record('closing')[turn]
    fold.unplace(closing)
    fold.place(closing, '')
  }
  if (fold.facts.calls[turn]) delete fold.record('calls')[turn]
}
