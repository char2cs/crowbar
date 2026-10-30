import { ApiError } from '@/lib/errors/api-error'
import { report } from '@/lib/errors/report'
import {
  retractPrompt,
  stopChat,
  submitAgentPrompt,
} from '@/features/agent/api/agent-chat-commands'
import { attachFeed } from '@/features/agent/stores/chat-feed'
import {
  chatStore,
  setChatSelection,
  type ChatStoreState,
} from '@/features/agent/stores/chat-store'
import {
  newestRetractable,
  nextToSend,
  onRowsApplied,
  onSendFailed,
  onSending,
  onSent,
  refusal,
  retryFailed,
  retryRow,
  type OutboxItem,
  type SendFailure,
} from '@/features/agent/stores/lib/outbox'

/** What the composer hands over with a prompt. */
export interface PromptInput {
  text: string
  steer: boolean
  provider?: string
  model?: string
  effort?: string
}

export type SendResult = { ok: true } | { ok: false; error?: string }

const NO_ITEMS: OutboxItem[] = []
// The feed stays attached while a chat has prompts with no row yet (R17).
const feedHolds = new Map<string, () => void>()
// Esc on an in-flight send: answered once the send does.
const takeBackOnAck = new Map<string, (text: string | null) => void>()

function itemsOf(chatId: string): OutboxItem[] {
  return chatStore.getState().outbox[chatId] ?? NO_ITEMS
}

function update(chatId: string, change: (items: OutboxItem[]) => OutboxItem[]) {
  const before = itemsOf(chatId)
  const next = change(before)
  if (next === before) return
  const { [chatId]: _old, ...rest } = chatStore.getState().outbox
  chatStore.setState({ outbox: next.length > 0 ? { ...rest, [chatId]: next } : rest })
  if (next.length > 0 && !feedHolds.has(chatId)) feedHolds.set(chatId, attachFeed(chatId))
  if (next.length === 0) {
    feedHolds.get(chatId)?.()
    feedHolds.delete(chatId)
  }
}

const drop = (chatId: string, requestId: string) =>
  update(chatId, (items) => items.filter((item) => item.requestId !== requestId))

// randomUUID needs a secure context, which not every webview gives.
function mintRequestId(): string {
  if (typeof globalThis.crypto.randomUUID === 'function') return globalThis.crypto.randomUUID()
  const bytes = globalThis.crypto.getRandomValues(new Uint8Array(16))
  bytes[6] = ((bytes[6] ?? 0) & 0x0f) | 0x40
  bytes[8] = ((bytes[8] ?? 0) & 0x3f) | 0x80
  const hex = [...bytes].map((value) => value.toString(16).padStart(2, '0')).join('')
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`
}

function failureOf(error: unknown): SendFailure {
  if (error instanceof ApiError) {
    return { status: error.status, code: error.problem.code, message: error.problem.message }
  }
  return { message: error instanceof Error ? error.message : String(error) }
}

/** Sends the head when the rows say the chat can take it; never on a clock. */
function drain(chatId: string) {
  const { feeds, chats } = chatStore.getState()
  const feed = feeds[chatId]
  if (feed?.status !== 'live') return
  const head = nextToSend(itemsOf(chatId), feed, chats[chatId]?.terminalWait !== undefined)
  if (head) void send(chatId, head)
}

async function send(chatId: string, item: OutboxItem) {
  update(chatId, (items) => onSending(items, item.requestId))
  let failure: unknown
  try {
    await submitAgentPrompt(
      item.wsId,
      chatId,
      item.text,
      item.requestId,
      item.provider ?? '',
      item.model,
      item.effort,
    )
    // The 202 is the only word that a staged pick was taken.
    if (item.model !== undefined || item.effort !== undefined) {
      setChatSelection(chatId, item.model ?? '', item.effort ?? '')
    }
    update(chatId, (items) => onSent(items, item.requestId))
  } catch (error) {
    failure = error
    update(chatId, (items) => onSendFailed(items, item.requestId, failureOf(error)))
  }
  const waiting = takeBackOnAck.get(item.requestId)
  if (waiting) {
    takeBackOnAck.delete(item.requestId)
    waiting(await takeBack(item.wsId, chatId, item.requestId, item.text))
    return
  }
  const { code } = failureOf(failure)
  const stillHere = itemsOf(chatId).some((left) => left.requestId === item.requestId)
  // Busy waits for a row; a lost session is said by the prompt itself.
  if (failure && stillHere && code !== 'chat_busy' && code !== 'session_lost') {
    report(failure, { action: 'Couldn’t send the prompt' })
  }
}

/** Retracts `requestId`; its text when the composer should get it back. */
async function takeBack(
  wsId: string,
  chatId: string,
  requestId: string,
  unsentText?: string,
  turnId?: string,
): Promise<string | null> {
  try {
    const answer = await retractPrompt(wsId, chatId, requestId)
    drop(chatId, requestId)
    const output = turnId && chatStore.getState().feeds[chatId]?.facts.output[turnId]
    return answer.state === 'interrupted' && output ? null : answer.text
  } catch (error) {
    // Never seen: the send did not get through, so the text is only ours.
    if (error instanceof ApiError && error.status === 404) {
      drop(chatId, requestId)
      return unsentText ?? null
    }
    // Settled, or its turn closed first: it stays in the transcript.
    if (!(error instanceof ApiError && error.status === 409)) {
      report(error, { action: 'Couldn’t take the prompt back' })
    }
    return null
  }
}

let watching = false

// Only the rows and the chat's own entity advance the outbox, never a clock.
function watch() {
  if (watching) return
  watching = true
  chatStore.subscribe((state: ChatStoreState, prev: ChatStoreState) => {
    for (const chatId of Object.keys(state.outbox)) {
      const feed = state.feeds[chatId]
      const was = prev.feeds[chatId]
      const same = feed?.facts === was?.facts && feed?.status === was?.status
      if (same && state.chats[chatId] === prev.chats[chatId]) continue
      if (feed) update(chatId, (items) => onRowsApplied(items, feed.facts))
      drain(chatId)
    }
  })
}

/** Puts a prompt in the transcript now and sends it when the chat can take it. */
export function sendPrompt(wsId: string, chatId: string, input: PromptInput): SendResult {
  const text = input.text.trim()
  const refused = refusal(text)
  if (refused !== null) return refused ? { ok: false, error: refused } : { ok: false }
  watch()
  const item: OutboxItem = { ...input, text, wsId, requestId: mintRequestId(), state: 'queued' }
  update(chatId, (items) => [...items, item])
  drain(chatId)
  return { ok: true }
}

/** Esc in an empty composer; null when there is nothing to act on, else the
 *  text to put back (null when it stopped the turn instead). */
export function retractNewest(wsId: string, chatId: string): Promise<string | null> | null {
  const feed = chatStore.getState().feeds[chatId]
  if (!feed) return null
  const target = newestRetractable(itemsOf(chatId), feed)
  switch (target?.kind) {
    case 'unsent':
      drop(chatId, target.item.requestId)
      return Promise.resolve(target.item.text)
    case 'in_flight': {
      const { requestId } = target.item
      update(chatId, (items) =>
        items.map((item) =>
          item.requestId === requestId ? { ...item, retractOnAck: true } : item,
        ),
      )
      return new Promise((resolve) => takeBackOnAck.set(requestId, resolve))
    }
    case 'held':
      return takeBack(wsId, chatId, target.requestId, target.text, target.turnId)
    case 'stop':
      return stopChat(wsId, chatId).then(
        () => null,
        (error: unknown) => {
          report(error, { action: 'Couldn’t stop the turn' })
          return null
        },
      )
    default:
      return null
  }
}

/** Retry on a failed send, or on a row its prompt left behind (a fresh request). */
export function retryPrompt(wsId: string, chatId: string, requestId: string, steer: boolean) {
  const freshId = mintRequestId()
  const feed = chatStore.getState().feeds[chatId]
  const row = feed?.items[`p:${requestId}`]
  watch()
  update(chatId, (items) => {
    const retried = retryFailed(items, requestId, freshId)
    if (retried !== items || row?.kind !== 'user' || !feed) return retried
    const fresh: OutboxItem = { requestId: freshId, wsId, text: row.text, state: 'queued', steer }
    return retryRow(items, feed.facts, row, fresh)
  })
  drain(chatId)
}

/** Takes a queued or failed prompt out; Edit is this plus its text in the composer. */
export function discardPrompt(chatId: string, requestId: string) {
  update(chatId, (items) =>
    items.filter((item) => item.requestId !== requestId || item.state === 'sending'),
  )
}

export function discardUnsent(chatId: string) {
  update(chatId, (items) => items.filter((item) => item.state === 'sending'))
}

/** The chat is gone: so is everything it had waiting. */
export function forgetOutbox(chatId: string) {
  update(chatId, () => NO_ITEMS)
}
