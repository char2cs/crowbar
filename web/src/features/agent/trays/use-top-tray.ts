import { useMemo } from 'react'
import { useShallow } from 'zustand/react/shallow'
import type { AgentChoice } from '@/features/agent/api/agent-chat-activity'
import type { PlanItem } from '@/features/agent/api/chat-rows'
import { useChatStore } from '@/features/agent/stores/chat-store'
import type { OutboxItem } from '@/features/agent/stores/lib/outbox'
import type { ChatFeed } from '@/features/agent/stores/lib/transcript-items'

/** Decided asks after the newest message, oldest first. */
function resolvedSinceLastMessage(feed: ChatFeed | undefined): AgentChoice[] {
  const choices: AgentChoice[] = []
  for (let i = (feed?.order.length ?? 0) - 1; i >= 0; i--) {
    const item = feed?.items.get(feed.order[i]!)
    if (item?.kind === 'user') break
    if (item?.kind === 'ask' && !item.choice.pending) choices.unshift(item.choice)
  }
  return choices
}

export interface TopTrayState {
  /** The plan while a step is left; null when there is none or it is finished. */
  steps: PlanItem[] | null
  queued: OutboxItem[]
  resolved: AgentChoice[]
}

/** What the tray above the input box has to say, read from the plan, outbox and feed. */
export function useTopTray(chatId: string, plan: PlanItem[] | undefined): TopTrayState {
  const outbox = useChatStore((s) => s.outbox[chatId])
  const queued = useMemo(() => (outbox ?? []).filter((item) => item.state === 'queued'), [outbox])
  // A message in the outbox is the next one: the answers it follows are no longer news.
  const typing = (outbox?.length ?? 0) > 0
  const resolved = useChatStore(
    useShallow((s) => (typing ? [] : resolvedSinceLastMessage(s.feeds[chatId]))),
  )
  const unfinished = plan?.some((step) => step.status !== 'done')
  return { steps: unfinished ? (plan ?? null) : null, queued, resolved }
}
