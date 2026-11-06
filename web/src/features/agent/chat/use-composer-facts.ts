import { useMemo } from 'react'
import type { PlanItem } from '@/features/agent/api/chat-rows'
import { useShallow } from 'zustand/react/shallow'
import { useFeedStatus, useLiveReasoning } from '@/features/agent/hooks/use-chat-feed'
import { chatStore, useChatStore, type ChatStoreState } from '@/features/agent/stores/chat-store'
import type { ItemMap } from '@/features/agent/stores/lib/item-map'
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

function runningTool(order: string[], items: ItemMap, turn: string) {
  for (let i = order.length - 1; i >= Math.max(0, order.length - TOOL_LOOKBACK); i--) {
    const item = items.get(order[i]!)
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

const NO_ASKS: AskItem[] = []

/** Pending asks of every scope, oldest first; the same items while none changed. */
function pendingAsks(feed: ChatFeed | undefined): AskItem[] {
  const ids = feed?.facts.openAsks ?? []
  if (ids.length === 0) return NO_ASKS
  return ids
    .map((id) => feed!.items.get(id))
    .filter((item): item is AskItem => item?.kind === 'ask' && item.choice.pending)
}

/** What the chat's feed says about the turn, read once for the view's parts. Each
 *  part is its own narrow read, so a row that changes none of them renders nothing. */
export function useComposerFacts(chatId: string): ComposerFacts {
  const feedOf = (s: ChatStoreState) => s.feeds[chatId]
  const status = useFeedStatus(chatId)
  const reasoning = useLiveReasoning(chatId)
  const running = useChatStore((s) => feedOf(s)?.facts.running)
  const plan = useChatStore((s) => feedOf(s)?.facts.plan[''])
  const agents = useChatStore((s) => feedOf(s)?.facts.liveSubagents.length ?? 0)
  const compacting = useChatStore((s) => feedOf(s)?.facts.compacting ?? false)
  const stopTurn = useChatStore((s) => feedOf(s)?.facts.stopping)
  const efforts = useChatStore((s) => feedOf(s)?.facts.efforts)
  const effort = useMemo(() => Object.values(efforts ?? {}).at(-1) ?? '', [efforts])
  const liveTail = useChatStore((s) => feedOf(s)?.liveTail.length ?? 0)
  const empty = useChatStore((s) => !feedOf(s)?.order.length && !feedOf(s)?.liveTail.length)
  const last = useChatStore((s) => {
    const feed = feedOf(s)
    return feed?.items.get(feed.order.at(-1) ?? '')
  })
  const toolLabel = useChatStore((s) => {
    const feed = feedOf(s)
    const turn = feed?.facts.running?.turnId
    return feed && turn ? runningTool(feed.order, feed.items, turn) : undefined
  })
  const asks = useChatStore(useShallow((s) => pendingAsks(feedOf(s))))
  const enteredAt = useChatStore((s) => {
    const newest = s.outbox[chatId]?.at(-1)
    return newest?.state === 'sending' ? newest.enteredAt : undefined
  })
  const halted = useMemo(() => haltOf(last), [last])
  const working = running != null || liveTail > 0 || agents > 0
  return {
    status,
    working,
    active: working || enteredAt !== undefined,
    since: running?.startedAt ?? enteredAt,
    compacting,
    reasoning,
    plan,
    activeTodo: plan?.find((step) => step.status === 'active')?.text,
    toolLabel,
    agents,
    streaming: liveTail > 0 || (last?.kind === 'text' && !last.settled),
    stopping: running != null && stopTurn === running.turnId,
    asks,
    halted,
    latestEffort: effort,
    empty,
  }
}

const promptsBy = new WeakMap<string[], string[]>()

/** The person's own prompts in `chatId`, oldest first, for history recall: read
 *  when a recall asks, so a landing row does not re-render the composer. */
export function promptTexts(chatId: string): string[] {
  const feed = chatStore.getState().feeds[chatId]
  if (!feed) return []
  let texts = promptsBy.get(feed.order)
  if (!texts) {
    texts = []
    for (const id of feed.order) {
      const item = feed.items.get(id)
      if (item?.kind === 'user' && (item.origin === 'crowbar' || item.origin === 'provider')) {
        texts.push(item.text)
      }
    }
    promptsBy.set(feed.order, texts)
  }
  return texts
}
