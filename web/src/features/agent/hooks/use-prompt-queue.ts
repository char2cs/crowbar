import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  submitAgentPrompt,
  type AgentChatMessage,
  type AgentPromptResult,
} from '@/features/agent/api/agent-api'
import {
  canPersistPromptQueue,
  isPromptTextWithinLimit,
  loadPromptQueue,
  MAX_PROMPT_QUEUE_ITEMS,
  savePromptQueue,
  type PromptQueueItem,
} from '@/features/agent/lib/prompt-queue-persistence'
import { ApiError } from '@/lib/api'

const BUSY_RECHECK_MS = 1_000
const AWAITING_TERMINAL_HINT_MS = 6_000

function requestId(): string {
  if (typeof globalThis.crypto?.randomUUID === 'function') return globalThis.crypto.randomUUID()
  if (typeof globalThis.crypto?.getRandomValues !== 'function') {
    throw new Error('Secure request identity is unavailable in this webview.')
  }
  const bytes = globalThis.crypto.getRandomValues(new Uint8Array(16))
  bytes[6] = ((bytes[6] ?? 0) & 0x0f) | 0x40
  bytes[8] = ((bytes[8] ?? 0) & 0x3f) | 0x80
  const hex = [...bytes].map((value) => value.toString(16).padStart(2, '0'))
  return `${hex.slice(0, 4).join('')}-${hex.slice(4, 6).join('')}-${hex
    .slice(6, 8)
    .join('')}-${hex.slice(8, 10).join('')}-${hex.slice(10).join('')}`
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}

/** A queue item handed to the server and waiting to be proven delivered. Only
 *  these are resolvable by evidence; the rest are the user's. */
export function awaitingEvidence(item: PromptQueueItem): boolean {
  return (
    item.state === 'submitting' ||
    item.state === 'awaiting_turn' ||
    item.state === 'outcome_uncertain'
  )
}

function samePrompt(message: AgentChatMessage, prompt: PromptQueueItem): boolean {
  return (
    message.role === 'user' &&
    message.sequence > prompt.baselineSequence &&
    message.text.trim() === prompt.text.trim()
  )
}

export type EnqueueResult = { ok: true } | { ok: false; error: string }

export interface PromptQueueOptions {
  wsId: string
  chatId: string
  working: boolean
  live: boolean
  active: boolean
  visible: boolean
  turnRevision: number
  terminalWaiting: boolean
  settledPrompts?: string[]
  /** The ledger's newest sequence, read at dispatch time to baseline evidence. */
  getBaseline: () => number
  /** Ask the ledger to re-read. Called after every dispatch outcome. */
  refreshMessages: () => void
  onPromptSpawned: (result: AgentPromptResult) => void | Promise<void>
  onPromptDispatchStart?: () => void
  onPromptDispatchSettled?: () => void
  onRefreshChat: () => Promise<boolean>
  /** The provider answered 422 prompt_submit_unsupported. */
  onSubmitUnavailable: () => void
}

/**
 * The prompt queue: a strict FIFO with barriers.
 *
 * Only the head can move, and `awaiting_turn`, `failed` and `outcome_uncertain`
 * heads are deliberate barriers — a later prompt overtaking one would reorder a
 * conversation. Everything here exists to answer one question honestly: did the
 * provider actually receive this text? Crowbar never replays a prompt it cannot
 * prove was rejected.
 *
 * ORDERING INVARIANT: the busy barrier releases BEFORE the queue reconciles.
 * Both run off the same `working` edge, and reconciling first can retire the
 * head while the barrier still names it, which dispatches the next prompt
 * against a runner that has not yet gone idle.
 */
export function usePromptQueue(options: PromptQueueOptions) {
  const {
    wsId,
    chatId,
    working,
    live,
    active,
    visible,
    turnRevision,
    terminalWaiting,
    settledPrompts,
    getBaseline,
    refreshMessages,
    onPromptSpawned,
    onPromptDispatchStart,
    onPromptDispatchSettled,
    onRefreshChat,
    onSubmitUnavailable,
  } = options

  const initialQueue = useMemo(() => loadPromptQueue(wsId, chatId), [wsId, chatId])
  const [queue, setQueue] = useState<PromptQueueItem[]>(initialQueue)
  const queueRef = useRef(initialQueue)
  const [persistenceLost, setPersistenceLost] = useState(false)
  const [showAwaitingTerminalHint, setShowAwaitingTerminalHint] = useState(false)

  const [idleEpoch, setIdleEpoch] = useState(0)
  const idleEpochRef = useRef(0)
  const previousWorking = useRef(working)
  const previousTurnRevision = useRef(turnRevision)
  const dispatching = useRef('')

  const updateQueue = useCallback(
    (change: (items: PromptQueueItem[]) => PromptQueueItem[]) => {
      const next = change(queueRef.current)
      queueRef.current = next
      setQueue(next)
      setPersistenceLost(!savePromptQueue(wsId, chatId, next))
      return next
    },
    [wsId, chatId],
  )

  const mark = useCallback(
    (id: string, change: (item: PromptQueueItem) => PromptQueueItem) =>
      updateQueue((items) =>
        items.map((item) => (item.clientRequestId === id ? change(item) : item)),
      ),
    [updateQueue],
  )

  const cancelUnsentPrompts = useCallback(
    () =>
      updateQueue((items) =>
        items.filter((item) => item.state !== 'queued' && item.state !== 'failed'),
      ),
    [updateQueue],
  )

  const cancelableCount = queue.filter(
    (item) => item.state === 'queued' || item.state === 'failed',
  ).length

  const deliveryPending = queue.some(
    (item) => item.state === 'submitting' || item.state === 'awaiting_turn',
  )

  const awaitingHead = queue[0]?.state === 'awaiting_turn' ? queue[0] : undefined
  useEffect(() => {
    setShowAwaitingTerminalHint(false)
    if (!active || !visible || !live || working || !awaitingHead) return
    // The daemon has an answer, so this guess stays quiet: the terminal-wait
    // banner is saying the same thing, without the "may".
    if (terminalWaiting) return

    const attemptedAt = Date.parse(awaitingHead.submittedAt ?? awaitingHead.createdAt)
    const delay = Math.max(0, AWAITING_TERMINAL_HINT_MS - (Date.now() - attemptedAt))
    if (delay === 0) {
      setShowAwaitingTerminalHint(true)
      return
    }
    const timer = window.setTimeout(() => setShowAwaitingTerminalHint(true), delay)
    return () => window.clearTimeout(timer)
  }, [active, visible, live, working, awaitingHead, terminalWaiting])

  useEffect(() => {
    if (previousWorking.current && !working) {
      idleEpochRef.current += 1
      setIdleEpoch(idleEpochRef.current)
    }
    previousWorking.current = working
  }, [working])

  const releaseBusyBarrier = useCallback((clientRequestId: string, epoch: number) => {
    const head = queueRef.current[0]
    if (
      head?.clientRequestId !== clientRequestId ||
      head.state !== 'queued' ||
      head.waitForIdleEpoch !== epoch ||
      epoch <= idleEpochRef.current
    )
      return
    idleEpochRef.current = epoch
    setIdleEpoch(epoch)
  }, [])

  // The stream's revision advances for every server-folded turn frame. React may
  // batch a fast busy -> idle pair so `working` renders false both before and
  // after it; the later revision plus a false folded value is nevertheless an
  // authoritative idle edge and must release a chat_busy retry barrier.
  useEffect(() => {
    const advanced = turnRevision > previousTurnRevision.current
    previousTurnRevision.current = turnRevision
    if (!advanced || working) return
    const head = queueRef.current[0]
    if (head?.state === 'queued' && head.waitForIdleEpoch !== undefined) {
      releaseBusyBarrier(head.clientRequestId, head.waitForIdleEpoch)
    }
  }, [turnRevision, working, releaseBusyBarrier])

  // A blocked FIFO also survives reloads and websocket outages. In those cases
  // there may be no future local edge to observe, so re-read the aggregate until
  // it reports idle. This never sends the prompt; it only lets the normal FIFO
  // dispatcher retry the same id after authoritative server confirmation.
  const busyHead =
    queue[0]?.state === 'queued' && queue[0].waitForIdleEpoch !== undefined ? queue[0] : undefined
  useEffect(() => {
    if (!busyHead || !active || !visible || !live) return
    let cancelled = false
    let checking = false
    const check = async () => {
      if (checking) return
      checking = true
      try {
        const serverWorking = await onRefreshChat()
        if (!cancelled && !serverWorking) {
          releaseBusyBarrier(busyHead.clientRequestId, busyHead.waitForIdleEpoch ?? 0)
        }
      } catch {
        // Stay blocked and try again. A failed read is not evidence of idle.
      } finally {
        checking = false
      }
    }
    void check()
    const timer = window.setInterval(() => void check(), BUSY_RECHECK_MS)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [busyHead, active, visible, live, onRefreshChat, releaseBusyBarrier])

  const reconcile = useCallback(
    (authoritative: AgentChatMessage[]) => {
      updateQueue((items) => {
        let changed = false
        const remaining = items.filter((item) => {
          if (!awaitingEvidence(item)) return true
          const confirmed = authoritative.some((message) => samePrompt(message, item))
          if (confirmed) changed = true
          return !confirmed
        })
        return changed ? remaining : items
      })
    },
    [updateQueue],
  )

  // A delivery the daemon has RETIRED resolves its own queue item, and nothing
  // else ever will: a provider built-in is handled inside the CLI, so no user
  // message for it is coming to the ledger and `reconcile` can never fire on it.
  // Without this the FIFO head sits in awaiting_turn and blocks the composer for
  // the rest of the runner's life.
  useEffect(() => {
    if (!settledPrompts?.length) return
    const settled = new Set(settledPrompts)
    updateQueue((items) => {
      const remaining = items.filter(
        (item) => !(awaitingEvidence(item) && settled.has(item.clientRequestId)),
      )
      return remaining.length === items.length ? items : remaining
    })
  }, [settledPrompts, updateQueue])

  /** Evidence still outstanding, for the ledger's recovery walk. */
  const pendingEvidence = useCallback(() => queueRef.current.some(awaitingEvidence), [])
  const pendingBaselines = useCallback(
    () => queueRef.current.filter(awaitingEvidence).map((item) => item.baselineSequence),
    [],
  )
  const onRecoveryExhausted = useCallback(() => {
    updateQueue((items) =>
      items.map((item) =>
        awaitingEvidence(item)
          ? {
              ...item,
              error:
                item.error ??
                'Prompt confirmation is older than the automatic recovery window. Load earlier messages or inspect Terminal before retrying.',
            }
          : item,
      ),
    )
  }, [updateQueue])

  const dispatch = useCallback(
    async (item: PromptQueueItem) => {
      if (dispatching.current) return
      dispatching.current = item.clientRequestId
      onPromptDispatchStart?.()
      mark(item.clientRequestId, (current) => ({
        ...current,
        state: 'submitting',
        error: undefined,
        // A retry is the same logical delivery attempt. Moving this baseline
        // forward could hide late hook evidence from the first network call.
        baselineSequence:
          current.submittedAt === undefined ? getBaseline() : current.baselineSequence,
        submittedAt: current.submittedAt ?? new Date().toISOString(),
        waitForIdleEpoch: undefined,
      }))
      try {
        const result = await submitAgentPrompt(wsId, chatId, item.text, item.clientRequestId)
        // Success means the replacement TUI exists, not that the provider has
        // accepted the message. Keep this row as the FIFO head until user_prompt
        // appears in the authoritative ledger.
        mark(item.clientRequestId, (current) => ({ ...current, state: 'awaiting_turn' }))
        try {
          await onPromptSpawned(result)
        } catch {
          // Runner adoption is view reconciliation. A successful prompt must never
          // be replayed because the follow-up GET/attach failed.
        }
        refreshMessages()
      } catch (error) {
        if (error instanceof ApiError && error.status === 409) {
          const code = error.code
          const definitelyBusy =
            code === 'chat_busy' || (!code && /\b(busy|working)\b/i.test(error.message))
          if (code === 'request_already_accepted') {
            // Dedupe proved the original restart succeeded. There is nothing to
            // resend and no need for a runner result: wait for hook evidence.
            mark(item.clientRequestId, (current) => ({
              ...current,
              state: 'awaiting_turn',
              error: undefined,
            }))
            refreshMessages()
          } else if (definitelyBusy) {
            const waitForIdleEpoch = idleEpochRef.current + 1
            mark(item.clientRequestId, (current) => ({
              ...current,
              state: 'queued',
              waitForIdleEpoch,
              error:
                'The provider became busy before this prompt could start. It will wait for idle.',
            }))
          } else if (code === 'request_id_conflict') {
            mark(item.clientRequestId, (current) => ({
              ...current,
              state: 'failed',
              error:
                'This request identity belongs to different prompt text. Edit the prompt to create a new request.',
            }))
          } else {
            mark(item.clientRequestId, (current) => ({
              ...current,
              state: 'outcome_uncertain',
              error:
                error.message ||
                'Crowbar cannot prove whether the provider accepted this prompt. It was not retried.',
            }))
            refreshMessages()
          }
        } else {
          const definitive =
            error instanceof ApiError && [400, 404, 422, 424].includes(error.status)
          if (
            error instanceof ApiError &&
            error.status === 422 &&
            error.code === 'prompt_submit_unsupported'
          )
            onSubmitUnavailable()
          mark(item.clientRequestId, (current) => ({
            ...current,
            // A rejected fetch or unexpected server failure can happen after the
            // daemon accepted and spawned. Only explicit pre-dispatch statuses are
            // safe to call failed; everything else blocks replay until hook
            // evidence or a human decision resolves it.
            state: definitive ? 'failed' : 'outcome_uncertain',
            error: errorMessage(error),
          }))
          if (!definitive) refreshMessages()
        }
      } finally {
        dispatching.current = ''
        onPromptDispatchSettled?.()
      }
    },
    [
      wsId,
      chatId,
      mark,
      getBaseline,
      refreshMessages,
      onPromptSpawned,
      onPromptDispatchStart,
      onPromptDispatchSettled,
      onSubmitUnavailable,
    ],
  )

  // Only the FIFO head can move.
  useEffect(() => {
    const head = queue[0]
    if (!head || head.state !== 'queued' || working || !live || !active || !visible) return
    if (head.waitForIdleEpoch !== undefined && head.waitForIdleEpoch > idleEpoch) return
    void dispatch(head)
  }, [queue, working, live, active, visible, idleEpoch, dispatch])

  // A replacement CLI that disappears before user_prompt is not accepted. Keep
  // the same request identity but require a human retry; never silently resubmit.
  useEffect(() => {
    if (live) return
    const awaiting = queueRef.current.find(
      (item) => item.state === 'submitting' || item.state === 'awaiting_turn',
    )
    if (!awaiting) return
    mark(awaiting.clientRequestId, (item) => ({
      ...item,
      state: 'outcome_uncertain',
      error:
        'The provider exited before Crowbar observed prompt confirmation. Check the ledger or terminal before retrying.',
    }))
    refreshMessages()
  }, [live, mark, refreshMessages])

  const enqueue = useCallback(
    (raw: string): EnqueueResult => {
      const text = raw.trim()
      if (!isPromptTextWithinLimit(text)) {
        return {
          ok: false,
          error: text
            ? 'This prompt is too large to submit.'
            : 'Write a message before submitting.',
        }
      }
      if (queueRef.current.length >= MAX_PROMPT_QUEUE_ITEMS) {
        return {
          ok: false,
          error: `This chat already has ${MAX_PROMPT_QUEUE_ITEMS} pending prompts.`,
        }
      }
      let clientRequestId: string
      try {
        clientRequestId = requestId()
      } catch (error) {
        return { ok: false, error: errorMessage(error) }
      }
      const item: PromptQueueItem = {
        clientRequestId,
        text,
        state: 'queued',
        createdAt: new Date().toISOString(),
        baselineSequence: getBaseline(),
      }
      const next = [...queueRef.current, item]
      if (!canPersistPromptQueue(wsId, chatId, next)) {
        return {
          ok: false,
          error:
            'Pending prompt storage is full or unavailable. Wait for queued work to finish before adding more.',
        }
      }
      updateQueue(() => next)
      return { ok: true }
    },
    [wsId, chatId, getBaseline, updateQueue],
  )

  const remove = useCallback(
    (id: string) => updateQueue((items) => items.filter((item) => item.clientRequestId !== id)),
    [updateQueue],
  )

  const retry = useCallback(
    (id: string) =>
      mark(id, (item) => ({
        ...item,
        state: 'queued',
        error: undefined,
        waitForIdleEpoch: working ? idleEpochRef.current + 1 : undefined,
        // SAME clientRequestId: server dedupe can return the first successful result.
      })),
    [mark, working],
  )

  return {
    queue,
    persistenceLost,
    cancelableCount,
    deliveryPending,
    awaitingHead,
    showAwaitingTerminalHint,
    enqueue,
    remove,
    retry,
    cancelUnsentPrompts,
    reconcile,
    pendingEvidence,
    pendingBaselines,
    onRecoveryExhausted,
  }
}
