import type { ProblemCode } from '@/lib/errors/problem'
import type { ChatFeed, TurnFacts, UserItem } from '@/features/agent/stores/lib/transcript-items'

export interface OutboxItem {
  requestId: string
  wsId: string
  text: string
  state: 'queued' | 'sending' | 'failed'
  enteredAt?: string
  acked?: boolean
  error?: string
  code?: ProblemCode
  sameIdRetry?: boolean
  retractOnAck?: boolean
  retryOf?: string
  steer: boolean
  provider?: string
  model?: string
  effort?: string
}

export interface SendFailure {
  status?: number
  code?: ProblemCode
  message: string
}

const MAX_PROMPT_TEXT_BYTES = 64 * 1024

export const SESSION_LOST_TEXT = 'The session was lost. Start a new conversation to send this.'

const RECORDED_CODES = new Set<ProblemCode>(['request_outcome_uncertain', 'session_lost'])

const recorded = (code?: ProblemCode) => code !== undefined && RECORDED_CODES.has(code)
const unknownOutcome = (failure: SendFailure) =>
  failure.status === undefined || failure.status >= 500

export const mayBeHeld = (failure: SendFailure) => unknownOutcome(failure) || recorded(failure.code)

export function refusal(text: string): string | null {
  if (!text) return ''
  if (/!\[[^\]]*\]\(blob:/.test(text)) return 'Wait for the attached photo to finish uploading.'
  if (new TextEncoder().encode(text).byteLength > MAX_PROMPT_TEXT_BYTES) {
    return 'This prompt is too large to submit.'
  }
  return null
}

const refusedOutright = (item: OutboxItem) =>
  item.state === 'failed' && !item.sameIdRetry && !recorded(item.code)

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
  const busy = facts.running || feed.liveTail.length > 0
  const stopping = facts.stopping !== null && facts.stopping === facts.running?.turnId
  if (busy && (!head.steer || stopping)) return undefined
  const blocking = Object.values(facts.prompts).some(
    (fact) => fact.state === 'pending' && !(head.steer && fact.steer),
  )
  return blocking ? undefined : head
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

export function retryRow(
  items: OutboxItem[],
  facts: TurnFacts,
  row: UserItem,
  fresh: OutboxItem,
): OutboxItem[] {
  const fact = row.requestId ? facts.prompts[row.requestId] : undefined
  const ended = fact?.state === 'delivered' && facts.running?.turnId !== fact.turnId
  const state = fact?.state
  if (!ended && state !== 'settled' && state !== 'refused' && state !== 'retracted') return items
  if (items.some((item) => item.retryOf === row.requestId)) return items
  return [...items, { ...fresh, retryOf: row.requestId }]
}

export function onRowsApplied(items: OutboxItem[], facts: TurnFacts): OutboxItem[] {
  const left = items.filter((item) => !(item.requestId in facts.prompts))
  return left.length === items.length ? items : left
}

export type Retractable =
  | { kind: 'unsent'; item: OutboxItem }
  | { kind: 'in_flight'; item: OutboxItem }
  | { kind: 'held'; requestId: string; text: string }
  | { kind: 'stop' }

const spoke = (feed: ChatFeed, turnId: string) =>
  feed.facts.output[turnId] === true ||
  feed.liveTail.some((id) => feed.items.get(id)?.turnId === turnId)

export function promptSpoke(feed: ChatFeed | undefined, requestId: string) {
  const fact = feed?.facts.prompts[requestId]
  const turnId =
    fact?.state === 'delivered' || fact?.state === 'retracted' ? fact.turnId : undefined
  return feed !== undefined && turnId !== undefined && spoke(feed, turnId)
}

export function newestRetractable(items: OutboxItem[], feed: ChatFeed): Retractable | null {
  const newest = items.at(-1)
  if (newest?.state === 'sending') {
    if (!newest.acked) return { kind: 'in_flight', item: newest }
    return { kind: 'held', requestId: newest.requestId, text: newest.text }
  }
  if (newest?.state === 'failed' && (newest.sameIdRetry || recorded(newest.code))) {
    return { kind: 'held', requestId: newest.requestId, text: newest.text }
  }
  if (newest) return { kind: 'unsent', item: newest }
  const { running, prompts } = feed.facts
  let item: UserItem | undefined
  for (let at = feed.order.length - 1; at >= 0 && !item; at--) {
    const candidate = feed.items.get(feed.order[at]!)
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

const held = new WeakMap<TurnFacts['prompts'], boolean>()

export function deliveryPending(items: OutboxItem[] | undefined, facts: TurnFacts | undefined) {
  if (items?.some((item) => item.state === 'sending')) return true
  if (!facts) return false
  let pending = held.get(facts.prompts)
  if (pending === undefined) {
    pending = Object.values(facts.prompts).some((fact) => fact.state === 'pending')
    held.set(facts.prompts, pending)
  }
  return pending
}
