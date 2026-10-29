import { useCallback, useEffect, useLayoutEffect, useRef } from 'react'
import type { AgentPromptResult } from '@/features/agent/api/agent-chat-commands'
import type { ComposerFacts } from '@/features/agent/chat/use-composer-facts'
import { usePromptQueue } from '@/features/agent/hooks/use-prompt-queue'

interface ChatSendOptions {
  wsId: string
  chatId: string
  facts: ComposerFacts
  steerable: boolean
  live: boolean
  canSend: boolean
  active: boolean
  visible: boolean
  terminalWaiting: boolean
  onPromptSpawned?: (result: AgentPromptResult) => void | Promise<void>
  onPromptDispatchStart?: () => void
  onPromptDispatchSettled?: () => void
  onSubmitUnavailable: () => void
  onSelectionCommitted?: (model: string, effort: string) => void
  /** The pane's badges and switch guard live outside the view's subtree. */
  onQueueCountChange?: (count: number) => void
  onCancelableQueueCountChange?: (count: number) => void
  onDeliveryPendingChange?: (pending: boolean) => void
}

/**
 * The prompt queue, fed from the chat's feed until the outbox replaces both
 * (S2 T5). The feed is pushed, so there is nothing to re-read after a dispatch;
 * a busy recheck answers from the folded turn, with no request.
 */
export function useChatSend(options: ChatSendOptions) {
  const { facts } = options
  const factsRef = useRef(facts)
  useLayoutEffect(() => {
    factsRef.current = facts
  })
  const getBaseline = useCallback(() => factsRef.current.turnRevision, [])
  const refreshMessages = useCallback(() => {}, [])
  // Unknown until the feed is live: answer busy, which keeps the barrier held.
  const onRefreshChat = useCallback(
    () => Promise.resolve(factsRef.current.status !== 'live' || factsRef.current.working),
    [],
  )
  const prompts = usePromptQueue({
    wsId: options.wsId,
    chatId: options.chatId,
    working: facts.working,
    steerable: options.steerable,
    compacting: facts.compacting,
    live: options.live,
    canSend: options.canSend,
    active: options.active,
    visible: options.visible,
    turnRevision: facts.turnRevision,
    terminalWaiting: options.terminalWaiting,
    settledPrompts: facts.settledPrompts,
    abandonedPrompts: facts.abandonedPrompts,
    getBaseline,
    refreshMessages,
    onPromptSpawned: options.onPromptSpawned,
    onPromptDispatchStart: options.onPromptDispatchStart,
    onPromptDispatchSettled: options.onPromptDispatchSettled,
    onRefreshChat,
    onSubmitUnavailable: options.onSubmitUnavailable,
    onSelectionCommitted: options.onSelectionCommitted,
  })
  const { onQueueCountChange, onCancelableQueueCountChange, onDeliveryPendingChange } = options
  const queued = prompts.queue.length
  // react-doctor-disable-next-line react-doctor/no-pass-live-state-to-parent -- the queue is owned here until T5
  useEffect(() => onQueueCountChange?.(queued), [queued, onQueueCountChange])
  const { cancelableCount, deliveryPending } = prompts
  useEffect(
    // react-doctor-disable-next-line react-doctor/no-pass-live-state-to-parent -- see above
    () => onCancelableQueueCountChange?.(cancelableCount),
    [cancelableCount, onCancelableQueueCountChange],
  )
  useEffect(
    // react-doctor-disable-next-line react-doctor/no-pass-live-state-to-parent -- see above
    () => onDeliveryPendingChange?.(deliveryPending),
    [deliveryPending, onDeliveryPendingChange],
  )
  return prompts
}
