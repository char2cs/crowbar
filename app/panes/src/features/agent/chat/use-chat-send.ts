import { useMemo } from 'react'
import {
  discardPrompt,
  retractNewest,
  retryPrompt,
  sendPrompt,
  stopTurn,
  type PromptInput,
} from '@/features/agent/stores/chat-outbox'
import { useChatStore } from '@/features/agent/stores/chat-store'
import { deliveryPending, type OutboxItem } from '@/features/agent/stores/lib/outbox'
import type { PromptActions } from '@/features/agent/transcript/rows/user-row'

const NO_ITEMS: OutboxItem[] = []

export function useChatSend(
  wsId: string,
  chatId: string,
  steer: boolean,
  onEdit: (text: string) => void,
) {
  const outbox = useChatStore((s) => s.outbox[chatId] ?? NO_ITEMS)
  const sending = useChatStore((s) => deliveryPending(s.outbox[chatId], s.feeds[chatId]?.facts))
  const actions = useMemo<PromptActions>(
    () => ({
      retry: (requestId) => retryPrompt(wsId, chatId, requestId, steer),
      edit: (requestId, text) => {
        discardPrompt(chatId, requestId)
        onEdit(text)
      },
    }),
    [wsId, chatId, steer, onEdit],
  )
  return {
    queued: outbox.length,
    waiting: outbox.filter((item) => item.state === 'queued').length,
    sending,
    unsupported: outbox.some((item) => item.code === 'prompt_submit_unsupported'),
    actions,
    send: (input: Omit<PromptInput, 'steer'>) => sendPrompt(wsId, chatId, { ...input, steer }),
    retract: () => retractNewest(wsId, chatId),
    withdraw: async () => {
      const text = await (retractNewest(wsId, chatId) ?? null)
      if (text === null) await stopTurn(wsId, chatId)
      return text
    },
  }
}
