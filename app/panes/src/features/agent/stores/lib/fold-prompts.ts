import type { LedgerRow } from '@/features/agent/api/chat-rows'
import type { FeedFold } from '@/features/agent/stores/lib/fold-rows'
import {
  markerItemId,
  userItemId,
  type MarkerItem,
  type PromptFact,
} from '@/features/agent/stores/lib/transcript-items'

export function setPrompt(fold: FeedFold, requestId: string, fact: PromptFact) {
  fold.record('prompts')[requestId] = fact
}

export function applyUserMessage(fold: FeedFold, row: LedgerRow<'user_message'>) {
  const { requestId, text, origin, delivery, retryOf } = row.body
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
  if (fold.facts.decidedAsks.length > 0) fold.set('decidedAsks', [])
  if (!requestId) return
  if (retryOf && fold.facts.prompts[retryOf]?.state === 'refused') {
    setPrompt(fold, retryOf, { state: 'retried', by: requestId })
  }
  const running = fold.facts.running
  // A prompt steered into the running turn is delivered by its own row.
  const steered = row.turnId && running?.turnId === row.turnId
  setPrompt(
    fold,
    requestId,
    steered
      ? { state: 'delivered', turnId: row.turnId! }
      : { state: 'pending', seq: row.seq, ...(delivery === 'restart' && { launch: true }) },
  )
}

// A retracted prompt went back to the composer, so its row leaves the transcript.
export function applyPromptSettled(fold: FeedFold, row: LedgerRow<'prompt_settled'>) {
  const { requestId, consumed, reason } = row.body
  if (consumed)
    setPrompt(fold, requestId, {
      state: 'settled',
      ...(reason === 'provider_builtin' && { builtin: true }),
    })
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
  'from_record',
  'session_conflict',
  'warning',
])
const LAUNCH_DIVIDERS = new Set(['provider_switched', 'model_changed', 'effort_changed'])

// The ids a provider stamps on each repeat of the same warning (cf-ray, request id).
const STAMP = /\b(cf-ray|request[ _-]?id)\s*[:=]\s*[\w-]+/gi

const sameWarning = (detail: string | undefined) => (detail ?? '').replace(STAMP, '$1')

/** A CLI repeats some warnings on every connection; the chat says each once. */
export function warningShown(fold: FeedFold, detail: string | undefined) {
  const said = sameWarning(detail)
  return fold.placedOrder.some((id) => {
    const item = fold.item(id)
    return item?.kind === 'marker' && item.marker === 'warning' && sameWarning(item.detail) === said
  })
}

const LAUNCH_ORDER = ['provider_switched', 'model_changed', 'effort_changed']

/** One launch's switches read as one divider, and a switch made with nothing
 *  said since replaces the earlier one of its kind: a quiet chat reads as where
 *  it landed. `drawn` joins the launch divider placed just before it, if any. */
function joinLaunch(fold: FeedFold, drawn: string) {
  const order = fold.placedOrder
  const at = order.lastIndexOf(drawn)
  const head = fold.item<MarkerItem>(order[at - 1] ?? '')
  const item = fold.item<MarkerItem>(drawn)
  if (!item || head?.kind !== 'marker' || !LAUNCH_DIVIDERS.has(head.marker)) return
  const parts = (head.parts ?? [{ marker: head.marker, detail: head.detail }])
    .filter((part) => part.marker !== item.marker)
    .concat({ marker: item.marker, detail: item.detail })
    .sort((a, b) => LAUNCH_ORDER.indexOf(a.marker) - LAUNCH_ORDER.indexOf(b.marker))
  fold.put({ ...head, parts })
  fold.unplace(drawn)
}

/** A session lost and then continued from the record is one event, said by the continuation. */
function absorbLostSession(fold: FeedFold, drawn: string) {
  const order = fold.placedOrder
  const head = fold.item<MarkerItem>(order[order.lastIndexOf(drawn) - 1] ?? '')
  if (head?.kind === 'marker' && head.marker === 'session_lost') fold.unplace(head.id)
}

/** `/compact` and the compaction it ran are one event, said once with the command in the pill. */
function absorbCompactCommand(fold: FeedFold, drawn: string) {
  const order = fold.placedOrder
  const head = fold.item(order[order.lastIndexOf(drawn) - 1] ?? '')
  const item = fold.item<MarkerItem>(drawn)
  if (!item || head?.kind !== 'user' || head.text.trim() !== '/compact') return
  fold.put({ ...item, command: '/compact' })
  fold.unplace(head.id)
}

/** Draws a pill marker; a launch's lands above the prompt it carries, which
 *  the ledger records first so the new CLI can find it. A selection change
 *  takes no busy guard, so a queued prompt stays where it is. */
export function divider(fold: FeedFold, kind: string, draw: () => string) {
  if (!PILL_MARKERS.has(kind)) return
  let carried: string | undefined
  let newest = -1
  for (const [requestId, fact] of Object.entries(fold.facts.prompts)) {
    if (fact.state !== 'pending' || !fact.launch || fact.seq < newest) continue
    newest = fact.seq
    carried = userItemId(requestId, fact.seq)
  }
  const launch = LAUNCH_DIVIDERS.has(kind)
  const move = launch && carried !== undefined && fold.placed(carried)
  if (move) fold.unplace(carried!)
  const drawn = draw()
  if (launch) joinLaunch(fold, drawn)
  if (kind === 'from_record') absorbLostSession(fold, drawn)
  if (kind === 'compaction_ended') absorbCompactCommand(fold, drawn)
  if (move) fold.place(carried!, '')
}
