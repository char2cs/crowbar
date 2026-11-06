import type { LedgerRow } from '@/features/agent/api/chat-rows'
import type { FeedFold } from '@/features/agent/stores/lib/fold-rows'
import {
  markerItemId,
  userItemId,
  type PromptFact,
} from '@/features/agent/stores/lib/transcript-items'

export function setPrompt(fold: FeedFold, requestId: string, fact: PromptFact) {
  fold.record('prompts')[requestId] = fact
}

export function applyUserMessage(fold: FeedFold, row: LedgerRow<'user_message'>) {
  const { requestId, text, origin, delivery } = row.body
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
  // Its fact first: where it is placed depends on how it waits.
  if (requestId) {
    const running = fold.facts.running
    // A prompt steered into the running turn is delivered by its own row.
    const steered = row.turnId && running?.turnId === row.turnId
    setPrompt(
      fold,
      requestId,
      steered
        ? { state: 'delivered', turnId: row.turnId! }
        : {
            state: 'pending',
            seq: row.seq,
            ...(delivery === 'restart' && { launch: true }),
            ...(delivery === 'steer' && { steer: true }),
          },
    )
  }
  fold.place(id, '')
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

/** A turn its prompt's retract ended before it said anything leaves no trace. */
export function retractTurn(fold: FeedFold, row: LedgerRow<'turn_ended'>) {
  const requestId = fold.facts.running?.turnId === row.turnId && fold.facts.running?.requestId
  const turn = row.turnId ?? ''
  if (row.body.reason !== 'retracted' || !requestId || !turn) return
  if (fold.facts.output[turn])
    return setPrompt(fold, requestId, { state: 'retracted', turnId: turn })
  setPrompt(fold, requestId, { state: 'retracted' })
  fold.record('traceless')[turn] = { requestId, marker: markerItemId(row.seq) }
  fold.unplace(userItemId(requestId, row.seq))
}

/** Output after a retract: the provider ran the prompt, so its row and end show
 *  above that output, even when the composer already got the text back. */
export function reviveTurn(fold: FeedFold, turn: string) {
  const lost = fold.facts.traceless[turn]
  if (!lost) return
  delete fold.record('traceless')[turn]
  setPrompt(fold, lost.requestId, { state: 'delivered', turnId: turn })
  fold.place(userItemId(lost.requestId, 0), '')
  if (fold.item(lost.marker)) fold.place(lost.marker, '')
}

const PILL_MARKERS = new Set([
  'compaction_ended',
  'provider_switched',
  'model_changed',
  'effort_changed',
  'legacy_unreadable',
  'session_conflict',
])
const LAUNCH_DIVIDERS = new Set(['provider_switched', 'model_changed', 'effort_changed'])

/** Draws a pill marker; a launch's lands above the prompt it carries, which
 *  the ledger records first so the new CLI can find it. A selection change
 *  takes no busy guard, so a queued prompt stays where it is. */
export function divider(fold: FeedFold, kind: string, draw: () => void) {
  if (!PILL_MARKERS.has(kind)) return
  let carried: string | undefined
  let newest = -1
  for (const [requestId, fact] of Object.entries(fold.facts.prompts)) {
    if (fact.state !== 'pending' || !fact.launch || fact.seq < newest) continue
    newest = fact.seq
    carried = userItemId(requestId, fact.seq)
  }
  const move = LAUNCH_DIVIDERS.has(kind) && carried !== undefined && fold.placed(carried)
  if (move) fold.unplace(carried!)
  draw()
  if (move) fold.place(carried!, '')
}
