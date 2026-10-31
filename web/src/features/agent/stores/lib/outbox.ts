import type { ProblemCode } from '@/lib/errors/problem'
import type { ChatFeed, TurnFacts, UserItem } from '@/features/agent/stores/lib/transcript-items'

/** A prompt the daemon has no row for yet: memory only, gone with the app. */
export interface OutboxItem {
  requestId: string
  wsId: string
  text: string
  state: 'queued' | 'sending' | 'failed'
  /** The send answered 202: the daemon holds it and its row is on the way. */
  acked?: boolean
  error?: string
  code?: ProblemCode
  /** A network error or 5xx: the daemon dedupes a resend under the same id. */
  sameIdRetry?: boolean
  retractOnAck?: boolean
  /** The settled or refused prompt row this item sends again. */
  retryOf?: string
  /** The provider takes a prompt into a running turn. */
  steer: boolean
  /** The staged pick; `undefined` model/effort is "nothing staged", '' a pick of the default. */
  provider?: string
  model?: string
  effort?: string
}

/** How a send failed, as the outbox reads it. */
export interface SendFailure {
  /** Absent for a network error, which never reached the daemon's answer. */
  status?: number
  code?: ProblemCode
  message: string
}

const MAX_PROMPT_TEXT_BYTES = 64 * 1024

export const SESSION_LOST_TEXT = 'The session was lost. Start a new conversation to send this.'

// Refusals the daemon gives after it recorded the prompt's user_message row.
const RECORDED_CODES = new Set<ProblemCode>(['request_outcome_uncertain', 'session_lost'])

const recorded = (code?: ProblemCode) => code !== undefined && RECORDED_CODES.has(code)
const unknownOutcome = (failure: SendFailure) =>
  failure.status === undefined || failure.status >= 500

/** Whether the daemon may hold a prompt whose send failed this way. */
export const mayBeHeld = (failure: SendFailure) => unknownOutcome(failure) || recorded(failure.code)

/** Why `text` cannot be queued, '' for an empty box (nothing to say), null when it can. */
export function refusal(text: string): string | null {
  if (!text) return ''
  // A `blob:` image is this tab's optimistic preview, meaningless to the agent until it uploads.
  if (/!\[[^\]]*\]\(blob:/.test(text)) return 'Wait for the attached photo to finish uploading.'
  if (new TextEncoder().encode(text).byteLength > MAX_PROMPT_TEXT_BYTES) {
    return 'This prompt is too large to submit.'
  }
  return null
}

/** A failed send the daemon definitely refused: it holds nothing, so the queue moves on. */
const refusedOutright = (item: OutboxItem) =>
  item.state === 'failed' && !item.sameIdRetry && !recorded(item.code)

/** The first prompt not refused outright, when the feed says the chat can take it. */
export function nextToSend(
  items: OutboxItem[],
  feed: ChatFeed,
  terminalWait: boolean,
): OutboxItem | undefined {
  const head = items.find((item) => !refusedOutright(item))
  const { facts } = feed
  if (head?.state !== 'queued' || head.retractOnAck || facts.compacting || terminalWait) {
    return undefined
  }
  // A reply streaming ahead of its rows is a turn the rows have not reported yet.
  if ((facts.running || feed.liveTail.length > 0) && !head.steer) return undefined
  if (Object.values(facts.prompts).some((fact) => fact.state === 'pending')) return undefined
  return head
}

function mark(items: OutboxItem[], requestId: string, change: Partial<OutboxItem>) {
  const at = items.findIndex((item) => item.requestId === requestId)
  if (at < 0) return items
  const next = [...items]
  next[at] = { ...items[at]!, ...change }
  return next
}

export const onSending = (items: OutboxItem[], requestId: string) =>
  mark(items, requestId, { state: 'sending', error: undefined, code: undefined })

export const onSent = (items: OutboxItem[], requestId: string) =>
  mark(items, requestId, { acked: true })

export function onSendFailed(items: OutboxItem[], requestId: string, failure: SendFailure) {
  if (failure.code === 'chat_busy') return mark(items, requestId, { state: 'queued' })
  return mark(items, requestId, {
    state: 'failed',
    acked: false,
    code: failure.code,
    sameIdRetry: unknownOutcome(failure),
    error: failure.code === 'session_lost' ? SESSION_LOST_TEXT : failure.message,
  })
}

/** A failed item queues again: under its own id when the daemon dedupes it, else a fresh one. */
export function retryFailed(items: OutboxItem[], requestId: string, freshId: string) {
  const item = items.find((candidate) => candidate.requestId === requestId)
  if (item?.state !== 'failed') return items
  return mark(items, requestId, {
    requestId: item.sameIdRetry ? requestId : freshId,
    state: 'queued',
    error: undefined,
    code: undefined,
    sameIdRetry: undefined,
  })
}

/** A prompt the rows abandoned or finished sends again as a new request; the row stays. */
export function retryRow(
  items: OutboxItem[],
  facts: TurnFacts,
  row: UserItem,
  fresh: OutboxItem,
): OutboxItem[] {
  const state = row.requestId ? facts.prompts[row.requestId]?.state : undefined
  if (state !== 'settled' && state !== 'refused' && state !== 'retracted') return items
  if (items.some((item) => item.retryOf === row.requestId)) return items
  return [...items, { ...fresh, retryOf: row.requestId }]
}

/** Lets go of every item whose user_message landed: from there the rows own it. */
export function onRowsApplied(items: OutboxItem[], facts: TurnFacts): OutboxItem[] {
  const left = items.filter((item) => !(item.requestId in facts.prompts))
  return left.length === items.length ? items : left
}

export type Retractable =
  /** Never sent, or refused: it leaves, and its text goes back. */
  | { kind: 'unsent'; item: OutboxItem }
  /** The send is in flight: retract once it answers. */
  | { kind: 'in_flight'; item: OutboxItem }
  /** The daemon holds it and no output came of it yet. */
  | { kind: 'held'; requestId: string; text: string }
  /** Its turn has output, or a turn runs that no prompt of ours started. */
  | { kind: 'stop' }

/** Whether `turnId` showed output: its rows, or a reply streaming ahead of them. */
const spoke = (feed: ChatFeed, turnId: string) =>
  feed.facts.output[turnId] === true ||
  feed.liveTail.some((id) => feed.items[id]?.turnId === turnId)

/** Whether the turn `requestId` started showed output: then its row is the record. */
export function promptSpoke(feed: ChatFeed | undefined, requestId: string) {
  const fact = feed?.facts.prompts[requestId]
  const turnId =
    fact?.state === 'delivered' || fact?.state === 'retracted' ? fact.turnId : undefined
  return feed !== undefined && turnId !== undefined && spoke(feed, turnId)
}

/** What Esc in an empty composer acts on: the newest prompt. */
export function newestRetractable(items: OutboxItem[], feed: ChatFeed): Retractable | null {
  const newest = items.at(-1)
  if (newest?.state === 'sending') {
    if (!newest.acked) return { kind: 'in_flight', item: newest }
    return { kind: 'held', requestId: newest.requestId, text: newest.text }
  }
  // A failed send the daemon may have recorded is retracted, or it would show twice.
  if (newest?.state === 'failed' && (newest.sameIdRetry || recorded(newest.code))) {
    return { kind: 'held', requestId: newest.requestId, text: newest.text }
  }
  if (newest) return { kind: 'unsent', item: newest }
  const { running, prompts } = feed.facts
  let item: UserItem | undefined
  for (let at = feed.order.length - 1; at >= 0 && !item; at--) {
    const candidate = feed.items[feed.order[at]!]
    if (candidate?.kind === 'user' && candidate.origin === 'crowbar') item = candidate
  }
  const fact = item?.requestId ? prompts[item.requestId] : undefined
  if (item?.requestId && fact?.state === 'pending') {
    return { kind: 'held', requestId: item.requestId, text: item.text }
  }
  if (item?.requestId && fact?.state === 'delivered' && fact.turnId === running?.turnId) {
    if (spoke(feed, fact.turnId)) return { kind: 'stop' }
    return { kind: 'held', requestId: item.requestId, text: item.text }
  }
  return running ? { kind: 'stop' } : null
}

/** A prompt is on its way: sending, or held by the daemon until a turn takes it. */
export function deliveryPending(items: OutboxItem[] | undefined, facts: TurnFacts | undefined) {
  if (items?.some((item) => item.state === 'sending')) return true
  return Object.values(facts?.prompts ?? {}).some((fact) => fact.state === 'pending')
}
