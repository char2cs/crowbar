import { useCallback, useEffect, useImperativeHandle, useRef, useState } from 'react'
import type { KeyboardEvent, Ref } from 'react'
import {
  compactChat,
  stopChat,
  type AgentChatMessage,
  type AgentPromptResult,
  type AgentProvider,
  type SlashCatalogItem,
} from '@/features/agent/api/agent-api'
import type { PromptQueueItem } from '@/features/agent/lib/prompt-queue-persistence'
import { blockedOn } from '@/features/agent/lib/agent-activity'
import { SubagentShelf } from '@/features/agent/activity/subagent-shelf'
import { AgentComposer } from '@/features/agent/composer/agent-composer'
import { ComposerSlashPicker } from '@/features/agent/composer/composer-slash-picker'
import { ProviderBar } from '@/features/agent/controls/provider-bar'
import { AgentTranscript } from '@/features/agent/transcript/agent-transcript'
import { AgentEmptyDocument } from '@/features/agent/chat/agent-empty-document'
import { useAgentActivity } from '@/features/agent/hooks/use-agent-activity'
import { useAgentTelemetry, limitResetsAt } from '@/features/agent/hooks/use-agent-telemetry'
import { useChatMessages } from '@/features/agent/hooks/use-chat-messages'
import { usePromptQueue } from '@/features/agent/hooks/use-prompt-queue'
import { useSlashCatalog } from '@/features/agent/hooks/use-slash-catalog'
import type { ChatPresentation } from '@/features/settings/lib/chat-presentation'

import '@/features/agent/styles/composer.css'
import '@/features/agent/styles/transcript.css'
import '@/features/agent/styles/activity.css'

export interface AgentChatViewHandle {
  cancelUnsentPrompts: () => void
}

export interface AgentChatViewProps {
  wsId: string
  chatId: string
  chatTitle?: string
  providerId: string
  providers: AgentProvider[]
  working: boolean
  /** Increments for every lifecycle frame, including a batched fast turn. */
  turnRevision: number
  live: boolean
  /** False while the native terminal presentation is selected. */
  active: boolean
  /** False for a retained, hidden tab. Network polling pauses in that state. */
  visible: boolean
  onOpenTerminal: () => void
  /** Whether the daemon has POSITIVELY established that this chat's CLI is
   *  blocked on a prompt Crowbar cannot answer. */
  terminalWaiting?: boolean
  terminalWaitKind?: string
  /** Client request ids the daemon has reported as delivered-and-over. */
  settledPrompts?: string[]
  streamingMessageId?: string
  streamingMessageText?: string
  onPromptSpawned: (result: AgentPromptResult) => void | Promise<void>
  onPromptDispatchStart?: () => void
  onPromptDispatchSettled?: () => void
  /** Re-read the server-folded busy value after a stale 409. */
  onRefreshChat: () => Promise<boolean>
  onQueueCountChange?: (count: number) => void
  onCancelableQueueCountChange?: (count: number) => void
  onDeliveryPendingChange?: (pending: boolean) => void
  /** The chat's sticky model / effort selection. '' means unset. */
  model: string
  effort: string
  onSelectionChange: (model: string, effort: string) => void
  /** Which surface the pane is showing, and how to change it. */
  presentation: ChatPresentation
  splitEnabled: boolean
  onSelectPresentation: (next: ChatPresentation) => void
  ref?: Ref<AgentChatViewHandle>
}

/** The provider's last word on why the chat stopped.
 *
 *  A `notice` is only the CURRENT state while nothing has happened after it — the
 *  next turn's own messages are the evidence that it no longer applies, so this
 *  reads the tail of the ledger rather than searching it. */
function haltedBy(messages: AgentChatMessage[]): AgentChatMessage | undefined {
  const last = messages.at(-1)
  return last?.role === 'notice' ? last : undefined
}

/**
 * The chat surface: a transcript, and one bar under it.
 *
 * This file ASSEMBLES. Every piece of state it used to own lives in a hook, and
 * every piece of markup in a component — what is left is the wiring between
 * them, which is the only thing that genuinely belongs to "the chat view".
 */
export function AgentChatView({
  wsId,
  chatId,
  chatTitle,
  providerId,
  providers,
  working,
  turnRevision,
  live,
  active,
  visible,
  onOpenTerminal,
  terminalWaiting = false,
  terminalWaitKind,
  settledPrompts,
  streamingMessageId,
  streamingMessageText,
  onPromptSpawned,
  onPromptDispatchStart,
  onPromptDispatchSettled,
  onRefreshChat,
  onQueueCountChange,
  onCancelableQueueCountChange,
  onDeliveryPendingChange,
  model,
  effort,
  onSelectionChange,
  presentation,
  splitEnabled,
  onSelectPresentation,
  ref,
}: AgentChatViewProps) {
  const activity = useAgentActivity(wsId, chatId, working, visible)
  const telemetry = useAgentTelemetry(wsId, chatId, visible)

  const [draft, setDraft] = useState('')
  const [fieldHeight, setFieldHeight] = useState(20)
  const fieldRef = useRef<HTMLTextAreaElement>(null)
  const [composerError, setComposerError] = useState('')
  const [submitUnavailable, setSubmitUnavailable] = useState(false)
  useEffect(() => setSubmitUnavailable(false), [chatId, providerId])

  // The queue baselines its evidence on the ledger's cursor and asks it to
  // re-read after every dispatch; the ledger's recovery walk asks the queue what
  // is still outstanding. Neither can be built before the other, so they meet
  // through this ref instead of through a hook order that cannot exist.
  const ledgerRef = useRef<{ cursor: number; refresh: () => void }>({
    cursor: 0,
    refresh: () => {},
  })
  const getBaseline = useCallback(() => ledgerRef.current.cursor, [])
  const refreshMessages = useCallback(() => ledgerRef.current.refresh(), [])
  const onSubmitUnavailable = useCallback(() => setSubmitUnavailable(true), [])

  const prompts = usePromptQueue({
    wsId,
    chatId,
    working,
    live,
    active,
    visible,
    turnRevision,
    terminalWaiting,
    settledPrompts,
    getBaseline,
    refreshMessages,
    onPromptSpawned,
    onPromptDispatchStart,
    onPromptDispatchSettled,
    onRefreshChat,
    onSubmitUnavailable,
  })

  const ledger = useChatMessages({
    wsId,
    chatId,
    providerId,
    visible,
    working,
    turnRevision,
    awaiting: prompts.deliveryPending,
    streamingMessageId,
    streamingMessageText,
    onApply: prompts.reconcile,
    pendingEvidence: prompts.pendingEvidence,
    pendingBaselines: prompts.pendingBaselines,
    onRecoveryExhausted: prompts.onRecoveryExhausted,
  })
  ledgerRef.current = { cursor: ledger.getCursor(), refresh: () => void ledger.refresh() }

  const slash = useSlashCatalog({ wsId, chatId, providerId, active, draft })

  useImperativeHandle(ref, () => ({ cancelUnsentPrompts: prompts.cancelUnsentPrompts }), [
    prompts.cancelUnsentPrompts,
  ])

  const { queue } = prompts
  useEffect(() => onQueueCountChange?.(queue.length), [queue.length, onQueueCountChange])
  useEffect(
    () => onCancelableQueueCountChange?.(prompts.cancelableCount),
    [prompts.cancelableCount, onCancelableQueueCountChange],
  )
  useEffect(
    () => onDeliveryPendingChange?.(prompts.deliveryPending),
    [prompts.deliveryPending, onDeliveryPendingChange],
  )

  const provider = providers.find((candidate) => candidate.id === providerId)
  const providerLabel = provider?.displayName ?? providerId
  // Compaction reaches the client as an unresolved interruption today and as a
  // chat work-state once the backend's inbound half lands. Reading the
  // interruption keeps this correct in both worlds — the state is additive.
  const compacting = blockedOn(activity)?.kind === 'compaction'
  // The provider's stop reason occupies the BAR, so the transcript must not also
  // render it as a row: it is one sentence, and saying it twice reads as the
  // provider having stopped twice.
  const halted = haltedBy(ledger.messages)

  const updateDraft = (value: string) => {
    setDraft(value)
    setComposerError('')
    slash.noteDraft(value)
  }

  const enqueueDraft = () => {
    const result = prompts.enqueue(draft)
    if (!result.ok) {
      setComposerError(result.error)
      return
    }
    setDraft('')
    setComposerError('')
    slash.reset()
  }

  const editPrompt = (item: PromptQueueItem) => {
    prompts.remove(item.clientRequestId)
    setDraft(item.text)
    setComposerError('')
    window.requestAnimationFrame(() => fieldRef.current?.focus())
  }

  const selectSlashItem = (item: SlashCatalogItem) => {
    setDraft(slash.accept(item))
    window.requestAnimationFrame(() => fieldRef.current?.focus())
  }

  const handleKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === 'Escape' && slash.open) {
      event.preventDefault()
      slash.close()
      return
    }
    if (
      slash.open &&
      slash.items.length > 0 &&
      (event.key === 'ArrowDown' || event.key === 'ArrowUp')
    ) {
      event.preventDefault()
      slash.move(event.key === 'ArrowDown' ? 1 : -1)
      return
    }
    // Tab is the completion key, and it is unconditional while something is
    // highlighted: it never sends, so it is the one keystroke a user can press
    // without first working out what the picker currently thinks it has.
    if (event.key === 'Tab' && !event.shiftKey && slash.highlighted) {
      event.preventDefault()
      selectSlashItem(slash.highlighted)
      return
    }
    if (event.key === 'Enter' && !event.shiftKey && !event.nativeEvent.isComposing) {
      event.preventDefault()
      // Enter accepts a completion when there IS one, and otherwise sends. It
      // used to be swallowed for as long as the picker was open, which does not
      // survive the catalog being INCOMPLETE BY DECLARATION: no probe reports a
      // provider's own built-ins, so /compact, /clear and /context match nothing
      // and never will. Esc still closes the picker outright.
      if (slash.highlighted) {
        selectSlashItem(slash.highlighted)
        return
      }
      enqueueDraft()
    }
  }

  return (
    <section className="agent-chat chat" aria-label="Agent chat">
      <AgentTranscript
        messages={ledger.messages}
        streamingBubble={ledger.streamingBubble}
        queue={queue}
        providers={providers}
        activity={activity}
        working={working && !compacting}
        loading={ledger.loading}
        error={ledger.error}
        hasOlder={ledger.hasOlder}
        onLoadOlder={() => void ledger.loadOlder()}
        onRetryLoad={() => void ledger.loadInitial()}
        onOpenTerminal={onOpenTerminal}
        onEditPrompt={editPrompt}
        onCancelPrompt={prompts.remove}
        onRetryPrompt={prompts.retry}
        showTerminalHintFor={
          prompts.showAwaitingTerminalHint ? prompts.awaitingHead?.clientRequestId : undefined
        }
        suppressSequence={halted?.sequence}
        empty={<AgentEmptyDocument title={chatTitle} />}
      />

      <div className="dock">
        <SubagentShelf activity={activity} />
        {slash.open && (
          <ComposerSlashPicker
            state={slash.state}
            items={slash.items}
            selected={slash.selected}
            onSelect={selectSlashItem}
          />
        )}
        <AgentComposer
          wsId={wsId}
          chatId={chatId}
          activity={activity}
          providerLabel={providerLabel}
          live={live}
          working={working}
          compacting={compacting}
          submitUnavailable={submitUnavailable}
          terminalWait={terminalWaiting ? { kind: terminalWaitKind ?? '' } : undefined}
          haltedMessage={halted?.text}
          haltedResetsAt={limitResetsAt(telemetry)}
          canStop={live}
          draft={draft}
          fieldHeight={fieldHeight}
          slashOpen={slash.open}
          onDraftChange={updateDraft}
          onHeightChange={setFieldHeight}
          onKeyDown={handleKeyDown}
          onSend={enqueueDraft}
          onStop={() => void stopChat(wsId, chatId)}
          onOpenTerminal={onOpenTerminal}
          fieldRef={fieldRef}
        />
        <ProviderBar
          wsId={wsId}
          chatId={chatId}
          provider={provider}
          model={model}
          effort={effort}
          telemetry={telemetry}
          presentation={presentation}
          splitEnabled={splitEnabled}
          compacting={compacting}
          onCompact={() => void compactChat(wsId, chatId)}
          onSelectionChange={onSelectionChange}
          onSelectPresentation={onSelectPresentation}
          showSwitcher={presentation !== 'terminal'}
          boxed={ledger.messages.length === 0 && queue.length === 0}
        />
        {(composerError || prompts.persistenceLost) && (
          <p className="meta" role="alert">
            {composerError ||
              'Pending prompts cannot be saved on this device. Keep Crowbar open until they finish.'}
          </p>
        )}
      </div>
    </section>
  )
}
