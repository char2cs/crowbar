import { ApiError } from '@/lib/errors/api-error'
import { report } from '@/lib/errors/report'
import { actionErrorMessage } from '@/features/hosts/lib/host-failure-notice'
import { toastManager } from '@/lib/toast-manager'
import {
  retractPrompt,
  stopChat,
  submitAgentPrompt,
} from '@/features/agent/api/agent-chat-commands'
import { attachFeed } from '@/features/agent/stores/chat-feed'
import { chatHostOf } from '@/lib/host-resolve'
import {
  addPromptStash,
  createPromptStash,
  loadPromptStashes,
  savePromptStashes,
} from '@/features/agent/composer/lib/prompt-stash-persistence'
import { chatStore, setStopping, type ChatStoreState } from '@/features/agent/stores/chat-store'
import { entityOf, entityStore, patchEntities } from '@/core/entities/entity-store'
import { entityKey } from '@/core/stream/host-scope'
import { forgetWithChat } from '@/features/agent/stores/chat-lifecycle'
import {
  mayBeHeld,
  newestRetractable,
  nextToSend,
  onRowsApplied,
  onSendFailed,
  onSending,
  onSent,
  promptSpoke,
  refusal,
  retryFailed,
  retryRow,
  type OutboxItem,
  type SendFailure,
} from '@/features/agent/stores/lib/outbox'

export interface PromptInput {
  text: string
  steer: boolean
  provider?: string
  model?: string
  effort?: string
}

export type SendResult = { ok: true } | { ok: false; error?: string }

const NO_ITEMS: OutboxItem[] = []
const feedHolds = new Map<string, () => void>()
interface Pending {
  promise: Promise<string | null>
  resolve: (text: string | null) => void
}
const takeBackOnAck = new Map<string, Pending>()
const terminalHolds = new Map<string, number>()

function itemsOf(chatId: string): OutboxItem[] {
  return chatStore.getState().outbox[chatId] ?? NO_ITEMS
}

function unsentById(items: OutboxItem[]) {
  const unsent = new Map<string, OutboxItem>()
  for (const item of items)
    if (item.state === 'failed' || item.state === 'queued')
      unsent.set(`unsent:${item.requestId}`, item)
  return unsent
}

function stashUnsent(chatId: string, before: OutboxItem[], next: OutboxItem[]) {
  const was = unsentById(before)
  const now = unsentById(next)
  const left = new Set([...was.keys()].filter((id) => !now.has(id)))
  const added = [...now].filter(([id]) => !was.has(id))
  const wsId = (added[0]?.[1] ?? was.get([...left][0] ?? ''))?.wsId
  if (!wsId) return
  const kept = loadPromptStashes(wsId, chatId).filter((stash) => !left.has(stash.id))
  const stashes = added.reduce((items, [id, item]) => {
    const stash = createPromptStash(item.text, { id })
    return stash ? addPromptStash(items, stash) : items
  }, kept)
  savePromptStashes(wsId, chatId, stashes)
}

function update(chatId: string, change: (items: OutboxItem[]) => OutboxItem[]) {
  const before = itemsOf(chatId)
  const next = change(before)
  if (next === before) return
  stashUnsent(chatId, before, next)
  const { [chatId]: _old, ...rest } = chatStore.getState().outbox
  chatStore.setState({ outbox: next.length > 0 ? { ...rest, [chatId]: next } : rest })
  const hostId = next.length > 0 && !feedHolds.has(chatId) ? chatHostOf(chatId) : null
  if (hostId) feedHolds.set(chatId, attachFeed(hostId, chatId))
  if (next.length === 0) {
    feedHolds.get(chatId)?.()
    feedHolds.delete(chatId)
  }
}

const drop = (chatId: string, requestId: string) =>
  update(chatId, (items) => items.filter((item) => item.requestId !== requestId))

function mintRequestId(): string {
  if (typeof globalThis.crypto.randomUUID === 'function') return globalThis.crypto.randomUUID()
  const bytes = globalThis.crypto.getRandomValues(new Uint8Array(16))
  bytes[6] = ((bytes[6] ?? 0) & 0x0f) | 0x40
  bytes[8] = ((bytes[8] ?? 0) & 0x3f) | 0x80
  const hex = [...bytes].map((value) => value.toString(16).padStart(2, '0')).join('')
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`
}

function failureOf(error: unknown): SendFailure {
  const message = actionErrorMessage(error, String(error))
  if (error instanceof ApiError) return { status: error.status, code: error.problem.code, message }
  return { message }
}

function drain(chatId: string) {
  const feed = chatStore.getState().feeds[chatId]
  if (feed?.status !== 'live' || terminalHolds.has(chatId)) return
  const hostId = chatHostOf(chatId)
  const chat = hostId ? entityOf(entityStore.getState(), 'chat', hostId, chatId) : undefined
  const waits = chat?.terminalWait !== undefined
  const head = nextToSend(itemsOf(chatId), feed, waits)
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
      item.retryOf,
    )
    const hostId = chatHostOf(chatId)
    if (hostId && (item.model !== undefined || item.effort !== undefined)) {
      const pick = { model: item.model ?? '', effort: item.effort ?? '' }
      patchEntities('chat', { [entityKey(hostId, chatId)]: pick })
    }
    update(chatId, (items) => onSent(items, item.requestId))
  } catch (error) {
    failure = error
    update(chatId, (items) => onSendFailed(items, item.requestId, failureOf(error)))
    if (failureOf(error).code !== 'chat_busy') drain(chatId)
  }
  const waiting = takeBackOnAck.get(item.requestId)
  if (waiting) {
    takeBackOnAck.delete(item.requestId)
    waiting.resolve(await answerEsc(chatId, item, failure))
    return
  }
  const { code } = failureOf(failure)
  const stillHere = itemsOf(chatId).some((left) => left.requestId === item.requestId)
  if (failure && stillHere && code !== 'chat_busy' && code !== 'session_lost') {
    report(failure, { action: 'Couldn’t send the prompt' })
  }
}

function answerEsc(chatId: string, item: OutboxItem, failure: unknown) {
  if (failure !== undefined && !mayBeHeld(failureOf(failure))) {
    drop(chatId, item.requestId)
    return Promise.resolve(item.text)
  }
  return takeBack(item.wsId, chatId, item.requestId, item.text)
}

async function takeBack(
  wsId: string,
  chatId: string,
  requestId: string,
  unsentText?: string,
): Promise<string | null> {
  try {
    const answer = await retractPrompt(wsId, chatId, requestId)
    drop(chatId, requestId)
    const spoke = promptSpoke(chatStore.getState().feeds[chatId], requestId)
    return answer.state === 'interrupted' && spoke ? null : answer.text
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) {
      drop(chatId, requestId)
      return unsentText ?? null
    }
    if (error instanceof ApiError && error.problem.code === 'prompt_with_provider') {
      toastManager.add({ type: 'error', title: WITH_PROVIDER, description: 'Stop ends its turn.' })
      return null
    }
    if (!(error instanceof ApiError && error.status === 409)) {
      report(error, { action: 'Couldn’t take the prompt back' })
    }
    return null
  }
}

const WITH_PROVIDER = 'Its provider already has this prompt'
function retracting(requestId: string, answer: Promise<string | null>) {
  const id = `retracting:${requestId}`
  toastManager.add({ id, type: 'loading', title: 'Taking the prompt back…', timeout: 0 })
  return answer.finally(() => toastManager.close(id))
}

let watching = false

function watch() {
  if (watching) return
  watching = true
  chatStore.subscribe((state: ChatStoreState, prev: ChatStoreState) => {
    for (const chatId of Object.keys(state.outbox)) {
      const feed = state.feeds[chatId]
      const was = prev.feeds[chatId]
      if (feed?.facts === was?.facts && feed?.status === was?.status) continue
      if (feed) update(chatId, (items) => onRowsApplied(items, feed.facts))
      drain(chatId)
    }
  })
  entityStore.subscribe((s, prev) => {
    if (s.byKind.chat === prev.byKind.chat) return
    for (const chatId of Object.keys(chatStore.getState().outbox)) {
      const hostId = chatHostOf(chatId)
      const key = hostId && entityKey(hostId, chatId)
      if (key && s.byKind.chat[key] !== prev.byKind.chat[key]) drain(chatId)
    }
  })
}

export function sendPrompt(wsId: string, chatId: string, input: PromptInput): SendResult {
  const text = input.text.trim()
  const refused = refusal(text)
  if (refused !== null) return refused ? { ok: false, error: refused } : { ok: false }
  watch()
  const item: OutboxItem = {
    ...input,
    text,
    wsId,
    requestId: mintRequestId(),
    state: 'queued',
    enteredAt: new Date().toISOString(),
  }
  update(chatId, (items) => [...items, item])
  drain(chatId)
  return { ok: true }
}

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
      const already = takeBackOnAck.get(requestId)
      if (already) return already.promise
      update(chatId, (items) =>
        items.map((item) =>
          item.requestId === requestId ? { ...item, retractOnAck: true } : item,
        ),
      )
      let resolve: (text: string | null) => void = () => {}
      const promise = new Promise<string | null>((settle) => (resolve = settle))
      takeBackOnAck.set(requestId, { promise, resolve })
      return retracting(requestId, promise)
    }
    case 'held':
      return retracting(target.requestId, takeBack(wsId, chatId, target.requestId, target.text))
    case 'stop':
      return stopTurn(wsId, chatId).then(() => null)
    default:
      return null
  }
}

export function stopTurn(wsId: string, chatId: string): Promise<void> {
  setStopping(chatId, true)
  const steered = steeredInto(chatId)
  if (steered.length === 0) return stop(wsId, chatId)
  return Promise.all(steered.map((p) => takeBack(wsId, chatId, p.requestId, p.text))).then(
    async (taken) => {
      await stop(wsId, chatId)
      for (const text of taken) if (text !== null) sendPrompt(wsId, chatId, { text, steer: false })
    },
  )
}

const stop = (wsId: string, chatId: string) =>
  stopChat(wsId, chatId).then(
    () => undefined,
    (error: unknown) => {
      setStopping(chatId, false)
      report(error, { action: 'Couldn’t stop the turn' })
    },
  )

function steeredInto(chatId: string): { requestId: string; text: string }[] {
  const feed = chatStore.getState().feeds[chatId]
  if (!feed?.facts.running) return []
  const held: { requestId: string; text: string }[] = []
  for (const id of feed.order) {
    const item = feed.items.get(id)
    if (item?.kind !== 'user' || !item.requestId) continue
    if (feed.facts.prompts[item.requestId]?.state === 'pending') {
      held.push({ requestId: item.requestId, text: item.text })
    }
  }
  return held
}

export function retryPrompt(wsId: string, chatId: string, requestId: string, steer: boolean) {
  const freshId = mintRequestId()
  const feed = chatStore.getState().feeds[chatId]
  const row = feed?.items.get(`p:${requestId}`)
  watch()
  update(chatId, (items) => {
    const retried = retryFailed(items, requestId, freshId)
    if (retried !== items || row?.kind !== 'user' || !feed) return retried
    const fresh: OutboxItem = { requestId: freshId, wsId, text: row.text, state: 'queued', steer }
    return retryRow(items, feed.facts, row, fresh)
  })
  drain(chatId)
}

export function discardPrompt(chatId: string, requestId: string) {
  update(chatId, (items) =>
    items.filter((item) => item.requestId !== requestId || item.state === 'sending'),
  )
}

export function discardUnsent(chatId: string) {
  update(chatId, (items) => items.filter((item) => item.state === 'sending'))
}

export function holdForTerminal(chatId: string): () => void {
  terminalHolds.set(chatId, (terminalHolds.get(chatId) ?? 0) + 1)
  let held = true
  return () => {
    if (!held) return
    held = false
    const left = (terminalHolds.get(chatId) ?? 1) - 1
    if (left > 0) terminalHolds.set(chatId, left)
    else terminalHolds.delete(chatId)
    drain(chatId)
  }
}

/** @internal */
export function forgetOutbox(chatId: string) {
  update(chatId, () => NO_ITEMS)
}
forgetWithChat(forgetOutbox)
