import { useMemo } from 'react'
import type { PlanItem } from '@/features/agent/api/chat-rows'
import {
  useChatItems,
  useChatOrder,
  useFeedStatus,
  useLiveReasoning,
  useLiveTail,
  useTurnFacts,
} from '@/features/agent/hooks/use-chat-feed'
import { useChatStore } from '@/features/agent/stores/chat-store'
import type {
  AskItem,
  ChatFeed,
  TranscriptItem,
} from '@/features/agent/stores/lib/transcript-items'
import { liveLabel } from '@/features/agent/transcript/lib/tool-label'

export interface ComposerFacts {
  status?: ChatFeed['status']
  /** A turn runs, a reply streams ahead of its rows, or a subagent works. */
  working: boolean
  /** Working, or a prompt is on its way: the working line shows. */
  active: boolean
  /** When the turn started, or when Enter was pressed before it did. */
  since?: string
  compacting: boolean
  reasoning?: string
  /** The main agent's plan. */
  plan?: PlanItem[]
  activeTodo?: string
  /** The live label of the running main-agent tool. */
  toolLabel?: string
  /** Subagents still running. */
  agents: number
  /** Answer text is streaming. */
  streaming: boolean
  /** Stop was pressed on the running turn. */
  stopping: boolean
  /** Pending asks of every scope, oldest first. */
  asks: AskItem[]
  /** The provider's last word on why the chat stopped, while nothing follows it. */
  halted?: { id: string; text: string }
  /** The person's own prompts, oldest first, for history recall. */
  userTexts: string[]
  /** The effort the newest finished turn reported. */
  latestEffort: string
  /** Nothing has been said in this chat yet. */
  empty: boolean
}

function haltOf(item: TranscriptItem | undefined): ComposerFacts['halted'] {
  if (item?.kind === 'user' && item.origin === 'note') return { id: item.id, text: item.text }
  if (item?.kind === 'marker' && item.marker === 'turn_failed') {
    return { id: item.id, text: item.detail || 'The provider stopped this turn.' }
  }
  return undefined
}

/** How far back from the newest row a running main tool is looked for. */
const TOOL_LOOKBACK = 50

function runningTool(
  order: string[],
  items: Record<string, TranscriptItem> | undefined,
  turn?: string,
) {
  for (let i = order.length - 1; i >= Math.max(0, order.length - TOOL_LOOKBACK); i--) {
    const item = items?.[order[i]!]
    if (item?.kind === 'user') continue
    if (item?.turnId !== turn) break
    if (
      item?.kind === 'tool' &&
      item.status === 'running' &&
      item.family !== 'meta' &&
      item.family !== 'plan'
    ) {
      return liveLabel(item)
    }
  }
  return undefined
}

/** What the chat's feed says about the turn, read once for the view's parts. */
export function useComposerFacts(chatId: string): ComposerFacts {
  const facts = useTurnFacts(chatId)
  const order = useChatOrder(chatId)
  const liveTail = useLiveTail(chatId)
  const items = useChatItems(chatId)
  const status = useFeedStatus(chatId)
  const reasoning = useLiveReasoning(chatId)
  const enteredAt = useChatStore((s) => {
    const newest = s.outbox[chatId]?.at(-1)
    return newest?.state === 'sending' ? newest.enteredAt : undefined
  })
  const openAsks = facts?.openAsks
  const asks = useMemo(
    () =>
      (openAsks ?? [])
        .map((id) => items?.[id])
        .filter((item): item is AskItem => item?.kind === 'ask' && item.choice.pending),
    [openAsks, items],
  )
  const running = facts?.running
  const fromOrder = useMemo(() => {
    const userTexts: string[] = []
    for (const id of order) {
      const item = items?.[id]
      if (item?.kind === 'user' && (item.origin === 'crowbar' || item.origin === 'provider')) {
        userTexts.push(item.text)
      }
    }
    const last = items?.[order.at(-1) ?? '']
    return {
      userTexts,
      halted: haltOf(last),
      toolLabel: running ? runningTool(order, items, running.turnId) : undefined,
      textStreaming: last?.kind === 'text' && !last.settled,
    }
  }, [order, items, running])
  const plan = facts?.plan['']
  const agents = facts?.liveSubagents.length ?? 0
  const working = running != null || liveTail.length > 0 || agents > 0
  return {
    status,
    working,
    active: working || enteredAt !== undefined,
    since: running?.startedAt ?? enteredAt,
    compacting: facts?.compacting ?? false,
    reasoning,
    plan,
    activeTodo: plan?.find((step) => step.status === 'active')?.text,
    toolLabel: fromOrder.toolLabel,
    agents,
    streaming: liveTail.length > 0 || fromOrder.textStreaming,
    stopping: running != null && facts?.stopping === running.turnId,
    asks,
    halted: fromOrder.halted,
    userTexts: fromOrder.userTexts,
    latestEffort: Object.values(facts?.efforts ?? {}).at(-1) ?? '',
    empty: order.length === 0 && liveTail.length === 0,
  }
}
