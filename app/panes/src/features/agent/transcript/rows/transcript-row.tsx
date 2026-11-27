import { memo } from 'react'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { useChatItem } from '@/features/agent/hooks/use-chat-feed'
import { useChatStore } from '@/features/agent/stores/chat-store'
import type { FailureActions, NoticeActions } from '@/features/agent/transcript/lib/marker-failure'
import { MarkerRow } from '@/features/agent/transcript/rows/marker-row'
import { TextRow } from '@/features/agent/transcript/rows/text-row'
import { AskRow, ToolRow } from '@/features/agent/transcript/rows/tool-row'
import { ToolRunRow } from '@/features/agent/transcript/rows/tool-run-row'
import { PlanRow } from '@/features/agent/transcript/rows/plan-row'
import { ReasoningRow } from '@/features/agent/transcript/rows/reasoning-row'
import { SubagentCard } from '@/features/agent/transcript/rows/subagent-card'
import { UserRow, type PromptActions } from '@/features/agent/transcript/rows/user-row'

export type RowActions = PromptActions & FailureActions & NoticeActions

export interface RowPlace {
  firstTurn?: boolean
  firstReply?: boolean
  turnbar?: boolean
}

function useTurnFact(
  chatId: string,
  turnId: string | undefined,
  fact: 'providers' | 'efforts' | 'models',
) {
  return useChatStore((s) => (turnId ? s.feeds[chatId]?.facts[fact][turnId] : undefined))
}

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
  members?: string[]
}) {
  const item = useChatItem(chatId, id)
  const provider = useTurnFact(chatId, item?.turnId, 'providers')
  const effort = useTurnFact(chatId, item?.turnId, 'efforts')
  const model = useTurnFact(chatId, item?.turnId, 'models')
  if (members) return <ToolRunRow chatId={chatId} id={id} members={members} />
  switch (item?.kind) {
    case undefined:
    case 'user':
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
          effort={effort}
          model={model}
          firstReply={place?.firstReply ?? false}
          turnbar={place?.turnbar ?? false}
        />
      )
    case 'tool':
      return <ToolRow chatId={chatId} item={item} />
    case 'ask':
      return <AskRow item={item} />
    case 'marker':
      return (
        <MarkerRow
          chatId={chatId}
          item={item}
          providers={providers}
          actions={actions}
          provider={provider}
        />
      )
    case 'subagent':
      return <SubagentCard chatId={chatId} id={id} />
    case 'reasoning':
      return <ReasoningRow chatId={chatId} item={item} />
    case 'plan':
      return <PlanRow item={item} />
  }
}

export const TranscriptRow = memo(TranscriptRowBody)
