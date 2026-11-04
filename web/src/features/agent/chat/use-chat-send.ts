import { useMemo } from 'react'
import {
  discardPrompt,
  retractNewest,
  retryPrompt,
  sendPrompt,
  type PromptInput,
} from '@/features/agent/stores/chat-outbox'
import { useChatStore } from '@/features/agent/stores/chat-store'
import { deliveryPending, type OutboxItem } from '@/features/agent/stores/lib/outbox'
import type { PromptActions } from '@/features/agent/transcript/rows/user-row'

const NO_ITEMS: OutboxItem[] = []

/** The view's side of the chat's outbox: Enter, Esc and the prompt rows' actions. */
export function useChatSend(
  wsId: string,
  chatId: string,
  steer: boolean,
  /** Puts a prompt's text in the composer; stable for the view's life. */
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
      cancel: (requestId) => discardPrompt(chatId, requestId),
    }),
    [wsId, chatId, steer, onEdit],
  )
  return {
    queued: outbox.length,
    /** Prompts still waiting their turn; a dispatched one is in flight, not queued. */
    waiting: outbox.filter((item) => item.state === 'queued').length,
    sending,
    /** The provider cannot take a typed prompt at all. */
    unsupported: outbox.some((item) => item.code === 'prompt_submit_unsupported'),
    actions,
    send: (input: Omit<PromptInput, 'steer'>) => sendPrompt(wsId, chatId, { ...input, steer }),
    retract: () => retractNewest(wsId, chatId),
  }
}
