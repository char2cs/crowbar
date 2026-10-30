import { memo } from 'react'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { useChatItem } from '@/features/agent/hooks/use-chat-feed'
import { useChatStore } from '@/features/agent/stores/chat-store'
import { MarkerRow } from '@/features/agent/transcript/rows/marker-row'
import { TextRow } from '@/features/agent/transcript/rows/text-row'
import { AskRow, ToolRow } from '@/features/agent/transcript/rows/tool-row'
import { UserRow, type PromptActions } from '@/features/agent/transcript/rows/user-row'

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
}: {
  chatId: string
  id: string
  providers: AgentProvider[]
  place?: RowPlace
  actions: PromptActions
}) {
  const item = useChatItem(chatId, id)
  const provider = useTurnFact(chatId, item?.turnId, 'providers')
  const effort = useTurnFact(chatId, item?.turnId, 'efforts')
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
      return <MarkerRow item={item} providers={providers} />
  }
}

export const TranscriptRow = memo(TranscriptRowBody)
