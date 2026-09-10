import { useCallback, useEffect, useRef, useState } from 'react'

import { type AgentActivity, listChatActivity } from '@/features/agent/api/agent-api'
import { NO_ACTIVITY, runningSubagents, runningTools } from '@/features/agent/lib/agent-activity'

/** How often a running turn's activity is re-read.
 *
 *  Activity has no push channel of its own: the chat's lifecycle frames announce
 *  that a turn started and ended, but a tool call starting mid-turn announces
 *  nothing. Polling only WHILE a turn runs is what keeps an idle chat silent. */
const POLL_MS = 1200

/** The falling-edge read's own retry spacing and budget — see its call site. */
const FALLING_EDGE_RETRY_MS = 400
const FALLING_EDGE_MAX_READS = 4

/** Read what the agent is doing, and what it did.
 *
 *  It reads ONCE when a chat becomes visible — a chat opened after its turns
 *  finished still has a timeline, and it would otherwise show none — then polls
 *  only while the chat is LIVE, with one final read on the falling edge. A chat
 *  nobody is looking at (`visible === false`) reads nothing at all.
 *
 *  Live is `working`, `compacting`, or a prompt still waiting on a human, because
 *  those are three different ways for the same chat to be unfinished. A pending
 *  prompt has to keep polling on its own account: it can stop pending without this
 *  client doing anything — somebody answers at the terminal, or the relay holding
 *  the CLI's gate times out and `answerable` goes false under a card still offering
 *  buttons. The prompts ride this payload, so that costs no second loop.
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
): AgentActivity {
  const [activity, setActivity] = useState<AgentActivity>(NO_ACTIVITY)
  const awaitingAnswer = activity.choices.some((choice) => choice.pending)
  const live = working || compacting || awaitingAnswer
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

  // The timeline of a chat that is ALREADY finished. Without this, opening a
  // completed chat shows a reply with none of the work that produced it.
  useEffect(() => {
    if (!visible) return
    const controller = new AbortController()
    void read(controller.signal)
    return () => controller.abort()
  }, [visible, read])

  // react-doctor-disable-next-line effect-needs-cleanup -- every path cleans up: the falling-edge branch's `cancelled` flag is checked immediately after each `await read(...)` before a next setTimeout is ever scheduled, and its own cleanup both clears fallingEdgeTimer.current and aborts the in-flight read; the two other branches return plain clearInterval/abort cleanups. Tracer can't follow a timer assigned inside a nested async closure.
  useEffect(() => {
    if (!visible) return
    const controller = new AbortController()
    const wasLive = previousLive.current
    previousLive.current = live

    if (!live) {
      // The falling edge. One read is not always enough: the hook's own comment
      // above already says the last tool completion can land AFTER `working`
      // flips, and this read can simply be the one that lands first. Losing that
      // race used to be permanent — polling stops the instant `live` goes false,
      // so a tool call caught still `running` here never got another chance to
      // settle, and stayed showing as active until some LATER, unrelated turn
      // gave activity a reason to poll again. Keep reading, briefly, for as long
      // as the response itself says something is still open.
      if (wasLive) {
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

    const timer = setInterval(() => void read(controller.signal), POLL_MS)
    return () => {
      clearInterval(timer)
      controller.abort()
    }
  }, [visible, live, read])

  return activity
}
