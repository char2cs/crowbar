import { useMemo } from 'react'
import type { PlanItem } from '@/features/agent/api/chat-rows'
import { useShallow } from 'zustand/react/shallow'
import { useFeedStatus, useLiveReasoning } from '@/features/agent/hooks/use-chat-feed'
import { chatStore, useChatStore, type ChatStoreState } from '@/features/agent/stores/chat-store'
import type { ItemMap } from '@/features/agent/stores/lib/item-map'
import {
  userItemId,
  type AskItem,
  type ChatFeed,
  type TranscriptItem,
} from '@/features/agent/stores/lib/transcript-items'
import { liveLabel } from '@/features/agent/transcript/lib/tool-label'
import type { WorkingLineProps } from '@/features/agent/activity/working-line'

export interface ComposerFacts {
  status?: ChatFeed['status']
  /** A turn runs, a reply streams ahead of its rows, or a subagent works. */
  working: boolean
  compacting: boolean
  /** The main agent's plan. */
  plan?: PlanItem[]
  /** Pending asks of every scope, oldest first. */
  asks: AskItem[]
  /** The provider's last word on why the chat stopped, while nothing follows it. */
  halted?: { id: string; text: string }
  /** The effort the newest finished turn reported. */
  latestEffort: string
  /** Nothing has been said in this chat yet. */
  empty: boolean
}

const lastOf = (feed: ChatFeed | undefined) => feed?.items.get(feed.order.at(-1) ?? '')

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

const heldBy = new WeakMap<ChatFeed['facts']['prompts'], string | null>()

/** When the oldest prompt the daemon holds for a CLI that has not taken it yet was
 *  said; cached on the prompts, which the fold copies only to change them. */
function heldSince(feed: ChatFeed | undefined): string | undefined {
  if (!feed) return undefined
  let since = heldBy.get(feed.facts.prompts)
  if (since === undefined) {
    since = null
    for (const [requestId, fact] of Object.entries(feed.facts.prompts)) {
      if (fact.state !== 'pending') continue
      const at = feed.items.get(userItemId(requestId, fact.seq))?.at
      if (at && (!since || at < since)) since = at
    }
    heldBy.set(feed.facts.prompts, since)
  }
  return since ?? undefined
}

/** What the chat's feed says about the turn for the view's parts. Each is its own
 *  narrow read, and none is a tool's or a token's, so a running turn renders nothing. */
export function useComposerFacts(chatId: string): ComposerFacts {
  const feedOf = (s: ChatStoreState) => s.feeds[chatId]
  const status = useFeedStatus(chatId)
  const working = useChatStore((s) => {
    const feed = feedOf(s)
    const { running, liveSubagents } = feed?.facts ?? {}
    return running != null || (feed?.liveTail.length ?? 0) > 0 || (liveSubagents?.length ?? 0) > 0
  })
  const plan = useChatStore((s) => feedOf(s)?.facts.plan[''])
  const compacting = useChatStore((s) => feedOf(s)?.facts.compacting ?? false)
  const efforts = useChatStore((s) => feedOf(s)?.facts.efforts)
  const effort = useMemo(() => Object.values(efforts ?? {}).at(-1) ?? '', [efforts])
  const empty = useChatStore((s) => !feedOf(s)?.order.length && !feedOf(s)?.liveTail.length)
  const haltId = useChatStore((s) => haltOf(lastOf(feedOf(s)))?.id)
  const haltText = useChatStore((s) => haltOf(lastOf(feedOf(s)))?.text)
  const asks = useChatStore(useShallow((s) => pendingAsks(feedOf(s))))
  const halted = useMemo(
    () => (haltId === undefined ? undefined : { id: haltId, text: haltText ?? '' }),
    [haltId, haltText],
  )
  return { status, working, compacting, plan, asks, halted, latestEffort: effort, empty }
}

/** What the working line says of the turn: these change with every tool and token,
 *  so only the line reads them. */
export function useWorkingLineFacts(chatId: string): WorkingLineProps {
  const feedOf = (s: ChatStoreState) => s.feeds[chatId]
  const reasoning = useLiveReasoning(chatId)
  const running = useChatStore((s) => feedOf(s)?.facts.running)
  const activeTodo = useChatStore(
    (s) => feedOf(s)?.facts.plan['']?.find((step) => step.status === 'active')?.text,
  )
  const agents = useChatStore((s) => feedOf(s)?.facts.liveSubagents.length ?? 0)
  const compacting = useChatStore((s) => feedOf(s)?.facts.compacting ?? false)
  const asking = useChatStore((s) => pendingAsks(feedOf(s)).length > 0)
  const stopTurn = useChatStore((s) => feedOf(s)?.facts.stopping)
  const liveTail = useChatStore((s) => feedOf(s)?.liveTail.length ?? 0)
  const textStreaming = useChatStore((s) => {
    const last = lastOf(feedOf(s))
    return last?.kind === 'text' && !last.settled
  })
  const toolLabel = useChatStore((s) => {
    const feed = feedOf(s)
    const turn = feed?.facts.running?.turnId
    return feed && turn ? runningTool(feed.order, feed.items, turn) : undefined
  })
  const enteredAt = useChatStore((s) => {
    const newest = s.outbox[chatId]?.at(-1)
    return newest?.state === 'sending' ? newest.enteredAt : undefined
  })
  // A send that launches a CLI lands its row long before the turn starts; it is still on its way.
  const held = useChatStore((s) => (feedOf(s)?.facts.running ? undefined : heldSince(feedOf(s))))
  const working = running != null || liveTail > 0 || agents > 0
  return {
    working: working || enteredAt !== undefined || held !== undefined,
    since: running?.startedAt ?? enteredAt ?? held,
    compacting,
    asking,
    activeTodo,
    toolLabel,
    agents,
    reasoning,
    streaming: liveTail > 0 || textStreaming,
    stopping: running != null && stopTurn === running.turnId,
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
