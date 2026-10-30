import type { LedgerRow } from '@/features/agent/api/chat-rows'
import type { FeedFold } from '@/features/agent/stores/lib/fold-rows'
import { userItemId, type PromptFact } from '@/features/agent/stores/lib/transcript-items'

export function setPrompt(fold: FeedFold, requestId: string, fact: PromptFact) {
  fold.record('prompts')[requestId] = fact
}

export function applyUserMessage(fold: FeedFold, row: LedgerRow<'user_message'>) {
  const { requestId, text, origin } = row.body
  const id = userItemId(requestId, row.seq)
  fold.put({
    kind: 'user',
    id,
    seq: row.seq,
    turnId: row.turnId,
    at: row.at,
    text,
    origin,
    requestId,
  })
  fold.place(id, '')
  if (!requestId) return
  const running = fold.facts.running
  // A prompt steered into the running turn is delivered by its own row.
  const steered = row.turnId && running?.turnId === row.turnId
  setPrompt(
    fold,
    requestId,
    steered ? { state: 'delivered', turnId: row.turnId! } : { state: 'pending', seq: row.seq },
  )
}

// A retracted prompt went back to the composer, so its row leaves the transcript.
export function applyPromptSettled(fold: FeedFold, row: LedgerRow<'prompt_settled'>) {
  const { requestId, consumed, reason } = row.body
  if (consumed) setPrompt(fold, requestId, { state: 'settled' })
  else if (reason === 'retracted') {
    setPrompt(fold, requestId, { state: 'retracted' })
    fold.unplace(userItemId(requestId, row.seq))
  } else setPrompt(fold, requestId, { state: 'refused', reason })
}

/** A turn its prompt's retract ended: true when it said nothing, so no trace stays. */
export function retractTurn(fold: FeedFold, row: LedgerRow<'turn_ended'>): boolean {
  const requestId = fold.facts.running?.turnId === row.turnId && fold.facts.running?.requestId
  if (row.body.reason !== 'retracted' || !requestId) return false
  setPrompt(fold, requestId, { state: 'retracted' })
  if (fold.facts.output[row.turnId ?? '']) return false
  fold.unplace(userItemId(requestId, row.seq))
  return true
}
