import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { listChatMessages, type AgentChatMessage } from '@/features/agent/api/agent-api'

const MESSAGE_PAGE_SIZE = 100
const MESSAGE_POLL_MS = 1_000
const EVIDENCE_RECOVERY_MAX_PAGES = 100
const EMPTY_MESSAGES: AgentChatMessage[] = []

function isAbort(error: unknown): boolean {
  return error instanceof DOMException && error.name === 'AbortError'
}

// Lets the main thread breathe between evidence-recovery pages so a chat
// opened with deep queued-evidence history (up to EVIDENCE_RECOVERY_MAX_PAGES
// pages) doesn't apply all of it as one uninterrupted synchronous task. A
// macrotask yield, not a frame — rAF never fires in an occluded/minimized
// webview, which would otherwise wedge this loop until foregrounded again.
function yieldToRenderer(): Promise<void> {
  // Called THROUGH `scheduler`, never as a detached reference — `yield` is a
  // WebIDL native method, brand-checked against its receiver, so
  // `scheduler.yield` extracted and invoked on its own throws "Illegal
  // invocation" (the same trap as destructuring navigator.clipboard.writeText).
  const scheduler = (globalThis as { scheduler?: { yield?: () => Promise<void> } }).scheduler
  return scheduler?.yield ? scheduler.yield() : new Promise((resolve) => setTimeout(resolve, 0))
}

// Sorted by displayOrder (dispatch order), NOT sequence (persist order) — an
// interrupted turn that finishes late must still display before a later
// turn that finished first. Falls back to sequence for anything predating
// the field (old fixtures only; every real response has it).
//
// The merge MAP's key is turnId, NOT sequence: a turn can legitimately be
// re-closed under a FRESH sequence for the SAME turnId (closeAssistantTurn's
// reconciliation loop always re-records the last streamed item to layer the
// terminating hook's effort/text onto it, even when nothing but that
// changed) — a real, reported duplicate ("Noted — saw the Codex exchange…"
// rendered twice, one copy missing `effort`, live 2026-08-29). Keying by
// sequence treated that second close as a brand-new row instead of an
// update to the first; turnId is the row's actual identity (every message
// IS its turn — see AgentChatMessage.turnId) and survives it.
function mergeMessages(current: AgentChatMessage[], incoming: AgentChatMessage[]) {
  const byTurnId = new Map(current.map((item) => [item.turnId, item]))
  for (const item of incoming) byTurnId.set(item.turnId, item)
  return [...byTurnId.values()].sort(
    (a, b) =>
      (a.displayOrder ?? a.sequence) - (b.displayOrder ?? b.sequence) ||
      (a.itemIndex ?? 0) - (b.itemIndex ?? 0),
  )
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
  /** The message(s) the agent is mid-way through saying. An array because a
   *  turn can have more than one open item (Codex; Claude is always 0-or-1)
   *  — see agent-chats-slice.ts. */
  streamingMessages?: { id: string; text: string }[]
  /** Ids from `streamingMessages` the ledger has now confirmed for real —
   *  see the `streamingBubbles` computation below, which this mirrors to
   *  prune the STORE side instead of just hiding the render. Their content
   *  now lives in `messages` too, so keeping the store's own copy around
   *  only grows `streamingMessages[chatId]` without bound over a long chat.
   *  Optional: a caller with no prune action (a test, a fixture) simply
   *  keeps the old behaviour. */
  onStreamingSettled?: (ids: string[]) => void
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
    streamingMessages,
    onStreamingSettled,
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
      if (incoming.length === 0) {
        // Nothing new: skip the Map rebuild and setMessages entirely — a
        // fresh array reference here forces AgentTranscript's messages.map()
        // to re-run on every unchanged poll tick. Cursor bookkeeping has
        // nothing to advance either. onApply still fires: it drives the
        // prompt-queue recovery walk in loadInitial, which reads
        // pendingEvidence()/recovery.hasMore after every applied page,
        // empty or not.
        onApply(messagesRef.current)
        return
      }
      const next = mergeMessages(messagesRef.current, incoming)
      messagesRef.current = next
      setMessages(next)
      cursorRef.current = Math.max(cursorRef.current, next.at(-1)?.sequence ?? 0)
      oldestCursorRef.current =
        oldestCursorRef.current === 0
          ? (next[0]?.sequence ?? 0)
          : Math.min(oldestCursorRef.current, next[0]?.sequence ?? oldestCursorRef.current)
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
      messagesRef.current = EMPTY_MESSAGES
      setMessages(EMPTY_MESSAGES)
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
          await yieldToRenderer()
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
          await yieldToRenderer()
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
      // False positive: this line IS the finally block's own body (see the
      // `finally {` immediately above) — it already runs on both the success and
      // rejection path.
      // react-doctor-disable-next-line react-doctor/no-loading-flag-reset-outside-finally
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

  // The message(s) being said right now, as bubbles below the recorded ones —
  // one per still-open item, in arrival order (Codex can have more than one
  // open per turn; Claude never does, so this is a single-entry array there,
  // same rendered output as before this array existed).
  //
  // Each is suppressed once the ledger has it. The two arrive from different
  // places — the live frame and the message poll — so for a moment both are
  // present, and rendering both would show the same sentence twice.
  //
  // Matched by id, not text: the backend can RECONCILE a streamed message's
  // text against the terminating hook's own copy when the two disagree
  // (turn/message.go's closeAssistantTurn — "its ok that the backend is
  // reconciling missing text, that's intended fallback behaviour"), and the
  // persisted text then legitimately differs from whatever was streamed. A
  // text comparison never matches that row, so the stale bubble is left on
  // screen underneath the real one forever — a real, reported duplicate. The
  // id survives reconciliation unchanged: assistantTurnID(messageID) is
  // always "msg-" + the SAME streamed message id regardless of whether its
  // text got replaced, so matching a ledger row's turnId against
  // "msg-"+this bubble's id is the reliable signal instead.
  const streamingBubbles = useMemo(() => {
    if (!streamingMessages?.length) return EMPTY_MESSAGES
    const bubbles: AgentChatMessage[] = []
    // Descending from MAX_SAFE_INTEGER, in arrival order, so a lone entry
    // gets the exact sentinel this used before arrays existed here, and
    // several entries still sort strictly after every real message.
    let sequence = Number.MAX_SAFE_INTEGER - (streamingMessages.length - 1)
    for (const m of streamingMessages) {
      const text = m.text.trim()
      const recordedTurnId = `msg-${m.id}`
      if (text && !messages.some((r) => r.role === 'assistant' && r.turnId === recordedTurnId)) {
        bubbles.push({ sequence, role: 'assistant', text: m.text, providerId, turnId: '', at: '' })
      }
      sequence++
    }
    return bubbles.length ? bubbles : EMPTY_MESSAGES
  }, [streamingMessages, messages, providerId])

  // The store-side twin of the suppression above: once a streamed message is
  // confirmed (same `"msg-" + id` match), its entry in streamingMessages[chatId]
  // is dead weight — the real copy lives in `messages` now. Recomputed on every
  // change rather than diffed against "previously confirmed", because
  // pruneAgentChatStreamingMessages is already a no-op when nothing in its id
  // list is still present, so a redundant call here costs a cheap early return,
  // never a wasted re-render loop.
  useEffect(() => {
    if (!streamingMessages?.length || !onStreamingSettled) return
    const confirmed = streamingMessages.reduce<string[]>((ids, m) => {
      if (messages.some((r) => r.role === 'assistant' && r.turnId === `msg-${m.id}`)) ids.push(m.id)
      return ids
    }, [])
    // onStreamingSettled prunes a SEPARATE store (streamingMessages[chatId]) this
    // hook does not own — there is no shared parent to lift into, only a prune
    // call to make in response to this hook's own derived state changing.
    // react-doctor-disable-next-line react-doctor/no-pass-live-state-to-parent
    if (confirmed.length > 0) onStreamingSettled(confirmed)
  }, [streamingMessages, messages, onStreamingSettled])

  const getCursor = useCallback(() => cursorRef.current, [])

  return {
    messages,
    hasOlder,
    loading,
    error,
    streamingBubbles,
    getCursor,
    loadInitial,
    refresh,
    loadOlder,
  }
}
