import { memo } from 'react'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { useChatItem } from '@/features/agent/hooks/use-chat-feed'
import { useChatStore } from '@/features/agent/stores/chat-store'
import { MarkerRow, type MarkerActions } from '@/features/agent/transcript/rows/marker-row'
import { TextRow } from '@/features/agent/transcript/rows/text-row'
import { AskRow, ToolRow } from '@/features/agent/transcript/rows/tool-row'
import { ToolRunRow } from '@/features/agent/transcript/rows/tool-run-row'
import { PlanRow } from '@/features/agent/transcript/rows/plan-row'
import { ReasoningRow } from '@/features/agent/transcript/rows/reasoning-row'
import { SubagentCard } from '@/features/agent/transcript/rows/subagent-card'
import { UserRow, type PromptActions } from '@/features/agent/transcript/rows/user-row'

/** Everything a transcript row can do. */
export type RowActions = PromptActions & MarkerActions

/** What a row's place in the conversation says about it, from the transcript. */
export interface RowPlace {
  firstTurn?: boolean
  firstReply?: boolean
  turnbar?: boolean
  precedingUserAt?: string
}

function useTurnFact(chatId: string, turnId: string | undefined, fact: 'providers' | 'efforts') {
  return useChatStore((s) => (turnId ? s.feeds[chatId]?.facts[fact][turnId] : undefined))
}

/** One transcript item, drawn by the component for its kind. */
function TranscriptRowBody({
  chatId,
  id,
  providers,
  place,
  actions,
  members,
}: {
  chatId: string
  id: string
  providers: AgentProvider[]
  place?: RowPlace
  actions: RowActions
  /** A run's calls, when `id` names a run. */
  members?: string[]
}) {
  const item = useChatItem(chatId, id)
  const provider = useTurnFact(chatId, item?.turnId, 'providers')
  const effort = useTurnFact(chatId, item?.turnId, 'efforts')
  if (members) return <ToolRunRow chatId={chatId} id={id} members={members} />
  switch (item?.kind) {
    case undefined:
    case 'user':
      // An id with no item yet is a prompt still in the outbox.
      if (!item && !id.startsWith('p:')) return null
      return (
        <UserRow
          chatId={chatId}
          id={id}
          item={item}
          providers={providers}
          provider={provider}
          firstTurn={place?.firstTurn ?? false}
          actions={actions}
        />
      )
    case 'text':
      return (
        <TextRow
          chatId={chatId}
          item={item}
          providers={providers}
          provider={provider}
          effort={effort}
          firstReply={place?.firstReply ?? false}
          turnbar={place?.turnbar ?? false}
          precedingUserAt={place?.precedingUserAt}
        />
      )
    case 'tool':
      return <ToolRow chatId={chatId} item={item} />
    case 'ask':
      return <AskRow item={item} />
    case 'marker':
      return <MarkerRow chatId={chatId} item={item} providers={providers} actions={actions} />
    case 'subagent':
      return <SubagentCard chatId={chatId} id={id} />
    case 'reasoning':
      return <ReasoningRow item={item} />
    case 'plan':
      return <PlanRow item={item} />
  }
}

export const TranscriptRow = memo(TranscriptRowBody)
