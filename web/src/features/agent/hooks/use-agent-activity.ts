import { useCallback, useEffect, useRef, useState } from 'react'

import { type AgentActivity, listChatActivity } from '@/features/agent/api/agent-api'
import { NO_ACTIVITY } from '@/features/agent/lib/agent-activity'

/** How often a running turn's activity is re-read.
 *
 *  Activity has no push channel of its own: the chat's lifecycle frames announce
 *  that a turn started and ended, but a tool call starting mid-turn announces
 *  nothing. Polling only WHILE a turn runs is what keeps an idle chat silent. */
const POLL_MS = 1200

/** Read what the agent is doing, and what it did.
 *
 *  It reads ONCE when a chat becomes visible — a chat opened after its turns
 *  finished still has a timeline, and it would otherwise show none — then polls
 *  only while the chat is LIVE, with one final read on the falling edge. A chat
 *  nobody is looking at (`visible === false`) reads nothing at all.
 *
 *  Live is `working` OR a prompt still waiting on a human, because those are two
 *  different ways for the same chat to be unfinished. A pending prompt has to keep
 *  polling on its own account: it can stop pending without this client doing
 *  anything — somebody answers at the terminal, or the relay holding the CLI's
 *  gate times out and `answerable` goes false under a card still offering buttons.
 *  The prompts ride this payload, so that costs no second loop.
 */
export function useAgentActivity(
  wsId: string,
  chatId: string,
  working: boolean,
  visible: boolean,
): AgentActivity {
  const [activity, setActivity] = useState<AgentActivity>(NO_ACTIVITY)
  const awaitingAnswer = activity.choices.some((choice) => choice.pending)
  const live = working || awaitingAnswer
  const previousLive = useRef(live)

  const read = useCallback(
    async (signal: AbortSignal) => {
      try {
        setActivity(await listChatActivity(wsId, chatId, { signal }))
      } catch {
        // Activity is a legibility surface, not the conversation. A failed read
        // leaves the last good timeline standing rather than blanking it.
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

  useEffect(() => {
    if (!visible) return
    const controller = new AbortController()
    const wasLive = previousLive.current
    previousLive.current = live

    if (!live) {
      // The falling edge: read once more so the finished turn is complete.
      if (wasLive) void read(controller.signal)
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
