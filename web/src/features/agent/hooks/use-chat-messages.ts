import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { listChatMessages, type AgentChatMessage } from '@/features/agent/api/agent-api'

const MESSAGE_PAGE_SIZE = 100
const MESSAGE_POLL_MS = 1_000
const EVIDENCE_RECOVERY_MAX_PAGES = 100

function isAbort(error: unknown): boolean {
  return error instanceof DOMException && error.name === 'AbortError'
}

function mergeMessages(current: AgentChatMessage[], incoming: AgentChatMessage[]) {
  const bySequence = new Map(current.map((item) => [item.sequence, item]))
  for (const item of incoming) bySequence.set(item.sequence, item)
  return [...bySequence.values()].sort((a, b) => a.sequence - b.sequence)
}

export interface ChatMessagesOptions {
  wsId: string
  chatId: string
  providerId: string
  visible: boolean
  working: boolean
  turnRevision: number
  /** A prompt is still waiting on hook confirmation, so keep polling. */
  awaiting: boolean
  streamingMessageId?: string
  streamingMessageText?: string
  /** Every applied page, for queue reconciliation. Called SYNCHRONOUSLY inside
   *  the recovery walk, which then re-reads `pendingEvidence` — an async
   *  reconciliation would make the walk read its own stale answer and page to
   *  the recovery budget every time. */
  onApply: (messages: AgentChatMessage[]) => void
  pendingEvidence: () => boolean
  pendingBaselines: () => number[]
  onRecoveryExhausted: () => void
}

/**
 * The authoritative ledger, paged.
 *
 * Messages are hook-confirmed records, never optimistic echoes: what a user
 * typed appears here only once the provider's own machinery reports it. That is
 * what makes the queue's evidence model work, and it is why this hook pages
 * BACKWARD on a fresh chat and FORWARD from a known baseline otherwise — the
 * confirming record can be arbitrarily old, and walking the whole history to
 * find it would cost a page per turn of the conversation.
 */
export function useChatMessages(options: ChatMessagesOptions) {
  const {
    wsId,
    chatId,
    providerId,
    visible,
    working,
    turnRevision,
    awaiting,
    streamingMessageId,
    streamingMessageText,
    onApply,
    pendingEvidence,
    pendingBaselines,
    onRecoveryExhausted,
  } = options

  const [messages, setMessages] = useState<AgentChatMessage[]>([])
  const messagesRef = useRef<AgentChatMessage[]>([])
  const cursorRef = useRef(0)
  const oldestCursorRef = useRef(0)
  const [hasOlder, setHasOlder] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<Error | null>(null)
  const loadGeneration = useRef(0)
  const refreshInFlight = useRef(false)
  const refreshAgain = useRef(false)

  const applyMessages = useCallback(
    (incoming: AgentChatMessage[]) => {
      const next = mergeMessages(messagesRef.current, incoming)
      messagesRef.current = next
      setMessages(next)
      if (next.length > 0) {
        cursorRef.current = Math.max(cursorRef.current, next.at(-1)?.sequence ?? 0)
        oldestCursorRef.current =
          oldestCursorRef.current === 0
            ? (next[0]?.sequence ?? 0)
            : Math.min(oldestCursorRef.current, next[0]?.sequence ?? oldestCursorRef.current)
      }
      onApply(next)
    },
    [onApply],
  )

  const loadInitial = useCallback(async () => {
    const generation = ++loadGeneration.current
    setLoading(true)
    setError(null)
    try {
      const page = await listChatMessages(wsId, chatId, { limit: MESSAGE_PAGE_SIZE })
      if (generation !== loadGeneration.current) return
      messagesRef.current = []
      cursorRef.current = page.cursor
      oldestCursorRef.current = page.oldestCursor
      setHasOlder(page.hasMore)
      applyMessages(page.items)

      if (!pendingEvidence()) return
      const baseline = Math.min(...pendingBaselines())
      let exhaustedRecoveryBudget = false

      if (baseline > 0) {
        // Ask forward from the evidence boundary. This avoids walking a long
        // history backward when the confirming hook is old but its exact baseline
        // is already persisted with the queue item.
        let after = baseline
        for (let index = 0; index < EVIDENCE_RECOVERY_MAX_PAGES; index++) {
          const recovery = await listChatMessages(wsId, chatId, {
            after,
            limit: MESSAGE_PAGE_SIZE,
          })
          if (generation !== loadGeneration.current) return
          applyMessages(recovery.items)
          if (!pendingEvidence() || !recovery.hasMore) return
          if (recovery.cursor <= after) return
          after = recovery.cursor
          exhaustedRecoveryBudget = index === EVIDENCE_RECOVERY_MAX_PAGES - 1
        }
      } else if (page.hasMore && page.oldestCursor > 0) {
        // A brand-new chat has baseline 0, which the paging API represents as
        // "no after cursor". Walk older pages only for that special case.
        let before = page.oldestCursor
        for (let index = 0; index < EVIDENCE_RECOVERY_MAX_PAGES; index++) {
          const recovery = await listChatMessages(wsId, chatId, {
            before,
            limit: MESSAGE_PAGE_SIZE,
          })
          if (generation !== loadGeneration.current) return
          applyMessages(recovery.items)
          if (!pendingEvidence() || !recovery.hasMore) return
          if (recovery.oldestCursor <= 0 || recovery.oldestCursor >= before) return
          before = recovery.oldestCursor
          exhaustedRecoveryBudget = index === EVIDENCE_RECOVERY_MAX_PAGES - 1
        }
      }

      if (exhaustedRecoveryBudget && pendingEvidence()) onRecoveryExhausted()
    } catch (err) {
      if (generation !== loadGeneration.current || isAbort(err)) return
      setError(err instanceof Error ? err : new Error(String(err)))
    } finally {
      if (generation === loadGeneration.current) setLoading(false)
    }
  }, [wsId, chatId, applyMessages, pendingEvidence, pendingBaselines, onRecoveryExhausted])

  const refresh = useCallback(async () => {
    if (refreshInFlight.current || loading) {
      refreshAgain.current = true
      return
    }
    refreshInFlight.current = true
    const generation = loadGeneration.current
    try {
      let after = cursorRef.current
      for (let index = 0; index < EVIDENCE_RECOVERY_MAX_PAGES; index++) {
        const page = await listChatMessages(wsId, chatId, { after, limit: MESSAGE_PAGE_SIZE })
        if (generation !== loadGeneration.current) return
        applyMessages(page.items)
        setError(null)
        if (!page.hasMore) break

        // `hasMore` is only actionable when the server advanced the forward
        // cursor. A stale/malformed page must not create an unbounded promise
        // recursion that pegs the renderer while repeatedly fetching the same
        // messages. Empty pages are covered by the same cursor check.
        if (page.cursor <= after) break
        after = page.cursor
      }
    } catch (err) {
      if (!isAbort(err) && generation === loadGeneration.current) {
        setError(err instanceof Error ? err : new Error(String(err)))
      }
    } finally {
      refreshInFlight.current = false
      if (refreshAgain.current && generation === loadGeneration.current) {
        refreshAgain.current = false
        void refresh()
      }
    }
  }, [wsId, chatId, loading, applyMessages])

  useEffect(() => {
    void loadInitial()
    return () => {
      loadGeneration.current += 1
    }
  }, [loadInitial])

  // A lifecycle frame changes the server-folded working value. Fetching on both
  // edges confirms the user and assistant hook messages. Poll while working (or
  // awaiting acceptance) because a turn_stop can legitimately leave Working true
  // while provider-reported async work continues.
  useEffect(() => {
    if (!visible) return
    void refresh()
    if (!working && !awaiting) return
    const timer = window.setInterval(() => void refresh(), MESSAGE_POLL_MS)
    return () => window.clearInterval(timer)
  }, [working, turnRevision, providerId, visible, awaiting, refresh])

  const loadOlder = useCallback(async () => {
    const before = oldestCursorRef.current
    if (!before) return
    try {
      const page = await listChatMessages(wsId, chatId, { before, limit: MESSAGE_PAGE_SIZE })
      applyMessages(page.items)
      oldestCursorRef.current = page.oldestCursor
      setHasOlder(page.hasMore)
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)))
    }
  }, [wsId, chatId, applyMessages])

  // The message being said right now, as a bubble below the recorded ones.
  //
  // Suppressed once the ledger has it. The two arrive from different places — the
  // live frame and the message poll — so for a moment both are present, and
  // rendering both would show the same sentence twice. Comparing the TEXT rather
  // than an id is what makes that work: the ledger row is keyed by the provider's
  // message id and this frame carries the same id, but the row is only written
  // when the message completes, so equal text is the earliest reliable signal
  // that the record has caught up.
  const streamingBubble = useMemo(() => {
    const text = streamingMessageText?.trim()
    if (!text || !streamingMessageId) return undefined
    if (messages.some((m) => m.role === 'assistant' && m.text.trim() === text)) return undefined
    return {
      sequence: Number.MAX_SAFE_INTEGER,
      role: 'assistant' as const,
      text: streamingMessageText ?? '',
      providerId,
      turnId: '',
      at: '',
    }
  }, [streamingMessageId, streamingMessageText, messages, providerId])

  const getCursor = useCallback(() => cursorRef.current, [])

  return {
    messages,
    hasOlder,
    loading,
    error,
    streamingBubble,
    getCursor,
    loadInitial,
    refresh,
    loadOlder,
  }
}
