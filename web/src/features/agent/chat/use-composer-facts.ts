import { useMemo } from 'react'
import {
  useChatItems,
  useChatOrder,
  useFeedStatus,
  useLiveReasoning,
  useLiveTail,
  useTurnFacts,
} from '@/features/agent/hooks/use-chat-feed'
import type {
  AskItem,
  ChatFeed,
  LiveSubagent,
  TranscriptItem,
  TurnFacts,
} from '@/features/agent/stores/lib/transcript-items'

export interface ComposerFacts {
  status?: ChatFeed['status']
  /** A turn runs, a reply streams ahead of its rows, or a subagent works. */
  working: boolean
  since?: string
  compacting: boolean
  reasoning?: string
  plan?: { text: string; status?: string }[]
  subagents: Record<string, LiveSubagent>
  /** Pending asks of every scope, oldest first. */
  asks: AskItem[]
  /** The provider's last word on why the chat stopped, while nothing follows it. */
  halted?: { id: string; text: string }
  /** The person's own prompts, oldest first, for history recall. */
  userTexts: string[]
  /** The effort the newest finished turn reported. */
  latestEffort: string
  /** How many turns have ended: the queue's revision and baseline. */
  turnRevision: number
  /** Prompts a turn took or the daemon settled: the queue lets them go. */
  settledPrompts: string[]
  /** Prompts refused with no proof the provider took them: the queue keeps their text. */
  abandonedPrompts: string[]
  /** Nothing has been said in this chat yet. */
  empty: boolean
}

const NO_SUBAGENTS: Record<string, LiveSubagent> = {}

function haltOf(item: TranscriptItem | undefined): ComposerFacts['halted'] {
  if (item?.kind === 'user' && item.origin === 'note') return { id: item.id, text: item.text }
  if (item?.kind === 'marker' && item.marker === 'turn_failed') {
    return { id: item.id, text: item.detail || 'The provider stopped this turn.' }
  }
  return undefined
}

function promptLists(prompts: TurnFacts['prompts'] | undefined) {
  const settled: string[] = []
  const abandoned: string[] = []
  for (const [requestId, fact] of Object.entries(prompts ?? {})) {
    if (fact.state === 'delivered' || fact.state === 'settled') settled.push(requestId)
    if (fact.state === 'refused') abandoned.push(requestId)
  }
  return { settled, abandoned }
}

/** What the chat's feed says about the turn, read once for the view's parts. */
export function useComposerFacts(chatId: string): ComposerFacts {
  const facts = useTurnFacts(chatId)
  const order = useChatOrder(chatId)
  const liveTail = useLiveTail(chatId)
  const items = useChatItems(chatId)
  const status = useFeedStatus(chatId)
  const reasoning = useLiveReasoning(chatId)
  const promptFacts = facts?.prompts
  const openAsks = facts?.openAsks
  const prompts = useMemo(() => promptLists(promptFacts), [promptFacts])
  const asks = useMemo(
    () =>
      (openAsks ?? [])
        .map((id) => items?.[id])
        .filter((item): item is AskItem => item?.kind === 'ask' && item.choice.pending),
    [openAsks, items],
  )
  const fromOrder = useMemo(() => {
    const userTexts: string[] = []
    for (const id of order) {
      const item = items?.[id]
      if (item?.kind === 'user' && (item.origin === 'crowbar' || item.origin === 'provider')) {
        userTexts.push(item.text)
      }
    }
    return { userTexts, halted: haltOf(items?.[order.at(-1) ?? '']) }
  }, [order, items])
  const plan = facts?.plan['']
  const planSteps = useMemo(() => plan?.map(({ text, status }) => ({ text, status })), [plan])
  const subagents = facts?.liveSubagents ?? NO_SUBAGENTS
  return {
    status,
    working: facts?.running != null || liveTail.length > 0 || Object.keys(subagents).length > 0,
    since: facts?.running?.startedAt,
    compacting: facts?.compacting ?? false,
    reasoning,
    plan: planSteps,
    subagents,
    asks,
    halted: fromOrder.halted,
    userTexts: fromOrder.userTexts,
    latestEffort: Object.values(facts?.efforts ?? {}).at(-1) ?? '',
    turnRevision: facts?.ended ?? 0,
    settledPrompts: prompts.settled,
    abandonedPrompts: prompts.abandoned,
    empty: order.length === 0 && liveTail.length === 0,
  }
}
