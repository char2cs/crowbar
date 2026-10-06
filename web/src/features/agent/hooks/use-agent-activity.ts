import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import {
  type AgentActivity,
  type AgentChoice,
  listChatActivity,
} from '@/features/agent/api/agent-api'
import {
  NO_ACTIVITY,
  runningSubagents,
  runningTools,
  withPendingChoices,
} from '@/features/agent/lib/agent-activity'

const NO_CHOICES: readonly AgentChoice[] = []

/** How often a running turn's activity is re-read.
 *
 *  Tool calls and subagents have no push channel of their own: the chat's
 *  lifecycle frames announce that a turn started and ended, but a tool call
 *  starting mid-turn announces nothing. Polling only WHILE a turn runs is what
 *  keeps an idle chat silent. Pending prompts DO have one (the `choice` frame),
 *  and are not read here. */
const POLL_MS = 1200

/** The falling-edge read's own retry spacing and budget — see its call site. */
const FALLING_EDGE_RETRY_MS = 400
const FALLING_EDGE_MAX_READS = 4

/** Read what the agent is doing, and what it did.
 *
 *  It reads ONCE when a chat is first shown — a chat opened after its turns
 *  finished still has a timeline, and it would otherwise show none — then polls
 *  only while the chat is LIVE and visible, with one final read on the falling
 *  edge even when hidden: a parked chat stays mounted and current, so showing it
 *  again costs no read unless its turn is still running.
 *
 *  The prompts the agent is blocked on are NOT read here: the daemon pushes them
 *  (`pending`, from the chat's `choice` frames) and they are laid over the read,
 *  so a card is there on the first frame a hidden chat is shown, however it got
 *  there. A prompt that stops pending — answered, expired, decided at the
 *  terminal, or its relay released — is a push too, and costs one read of the
 *  resolved record, taken on the same falling edge as a finished turn.
 *
 *  `compacting` counts too, even though it never opens a tracked turn (see
 *  compact.go): a compaction resolves its own interruption record durably, but
 *  nothing pushes that record live — only this hook's own falling-edge re-read
 *  picks it up, and `working` alone never sees the edge because it never moved.
 *  Without `compacting` here the finished compaction's divider stayed invisible
 *  until some LATER, unrelated live edge (the next prompt) forced a re-read.
 */
export function useAgentActivity(
  wsId: string,
  chatId: string,
  working: boolean,
  compacting: boolean,
  visible: boolean,
  turnRevision = 0,
  pending: readonly AgentChoice[] = NO_CHOICES,
): AgentActivity {
  const [recorded, setActivity] = useState<AgentActivity>(NO_ACTIVITY)
  const awaitingAnswer = pending.length > 0
  const polling = working || compacting
  const live = polling || awaitingAnswer
  const previousLive = useRef(live)
  // Written by the falling-edge poll below, cleared by that SAME effect run's own
  // cleanup — a ref rather than a closure-local `let` so the pending timer is
  // reachable from cleanup even though it is only assigned inside the async chain.
  const fallingEdgeTimer = useRef<ReturnType<typeof setTimeout>>(undefined)

  const read = useCallback(
    async (signal: AbortSignal): Promise<AgentActivity | undefined> => {
      try {
        const result = await listChatActivity(wsId, chatId, { signal })
        setActivity(result)
        return result
      } catch {
        // Activity is a legibility surface, not the conversation. A failed read
        // leaves the last good timeline standing rather than blanking it.
        return undefined
      }
    },
    [wsId, chatId],
  )

  // Reset on chat change: another chat's timeline must never appear under this
  // one while the first read is in flight.
  useEffect(() => {
    setActivity(NO_ACTIVITY)
  }, [chatId])

  // The timeline of a chat that is ALREADY finished, read when the chat is first
  // shown. Without this, opening a completed chat shows a reply with none of the
  // work that produced it. Showing it again reads nothing: a parked chat stays
  // mounted and the edges below keep its timeline current.
  const unread = recorded === NO_ACTIVITY
  useEffect(() => {
    if (!visible || !unread) return
    const controller = new AbortController()
    void read(controller.signal)
    return () => controller.abort()
  }, [visible, unread, read])

  const wasVisible = useRef(visible)
  const seenRevision = useRef(turnRevision)

  // react-doctor-disable-next-line effect-needs-cleanup -- every path cleans up: the falling-edge branch's `cancelled` flag is checked immediately after each `await read(...)` before a next setTimeout is ever scheduled, and its own cleanup both clears fallingEdgeTimer.current and aborts the in-flight read; the two other branches return plain clearInterval/abort cleanups. Tracer can't follow a timer assigned inside a nested async closure.
  useEffect(() => {
    const controller = new AbortController()
    const wasLive = previousLive.current
    const justShown = visible && !wasVisible.current && !unread
    // Moves with no working edge on a reconnect: the outage can hold a whole turn.
    const revisionMoved = turnRevision !== seenRevision.current
    previousLive.current = live
    wasVisible.current = visible
    seenRevision.current = turnRevision

    if (!live) {
      // The falling edge, taken whether or not the chat is on screen. One read is
      // not always enough: the hook's own comment
      // above already says the last tool completion can land AFTER `working`
      // flips, and this read can simply be the one that lands first. Losing that
      // race used to be permanent — polling stops the instant `live` goes false,
      // so a tool call caught still `running` here never got another chance to
      // settle, and stayed showing as active until some LATER, unrelated turn
      // gave activity a reason to poll again. Keep reading, briefly, for as long
      // as the response itself says something is still open.
      if (wasLive || revisionMoved) {
        let cancelled = false
        const poll = async (attempt: number) => {
          const result = await read(controller.signal)
          if (cancelled || !result) return
          const stillOpen = runningTools(result).length > 0 || runningSubagents(result) > 0
          if (stillOpen && attempt < FALLING_EDGE_MAX_READS) {
            fallingEdgeTimer.current = setTimeout(
              () => void poll(attempt + 1),
              FALLING_EDGE_RETRY_MS,
            )
          }
        }
        void poll(1)
        return () => {
          cancelled = true
          clearTimeout(fallingEdgeTimer.current)
          controller.abort()
        }
      }
      return () => controller.abort()
    }

    // Polling is for a chat somebody is looking at, and only while its turn runs:
    // a prompt alone is pushed. A running one shown again has missed whatever
    // tools started while it was parked, and nothing announces those, so that one
    // read is its catch-up (a first show has the read above).
    if (!visible || !polling) return () => controller.abort()
    if (justShown) void read(controller.signal)
    const timer = setInterval(() => void read(controller.signal), POLL_MS)
    return () => {
      clearInterval(timer)
      controller.abort()
    }
  }, [visible, live, polling, unread, turnRevision, read])

  return useMemo(() => withPendingChoices(recorded, pending), [recorded, pending])
}
