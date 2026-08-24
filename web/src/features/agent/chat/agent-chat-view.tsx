import { useCallback, useEffect, useImperativeHandle, useMemo, useRef, useState } from 'react'
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
import { SelectionCluster } from '@/features/agent/controls/selection-cluster'
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
  providerId: string
  providers: AgentProvider[]
  /** Move this chat to another provider — the identity chip's other groups. */
  onSwitchProvider?: (providerId: string) => Promise<boolean>
  /** A switch is already running, or the pane is mid-delivery. */
  switchDisabled?: boolean
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
  providerId,
  providers,
  onSwitchProvider,
  switchDisabled,
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
  // The box is UNCONTROLLED — a controlled contenteditable rebuilds itself under
  // the caret — so text pushed in from outside arrives by REMOUNT.
  //
  // THE SEED CARRIES ITS OWN TEXT rather than pointing at `draft`, because the
  // two are written by different things at nearly the same moment. The editor's
  // onChange lands asynchronously, so sending a prompt could bump the seed and
  // then have the outgoing text written back into `draft` before the new box
  // mounted — which mounted it holding the message that had just been sent.
  // Reading the seed's own text makes that ordering irrelevant.
  const [seed, setSeed] = useState({ text: '', n: 0 })
  // THE DOCK'S HEIGHT, published to CSS. The conversation runs UNDER the composer
  // and fades out behind it, so the scroller has to reserve exactly the room the
  // dock occupies — and the dock grows as the box does, so a constant would be
  // wrong the moment anybody typed a second line.
  const [dockHeight, setDockHeight] = useState(0)
  const dockObserver = useRef<ResizeObserver | null>(null)
  // A ref CALLBACK, not a ref + effect: the dock is unmounted entirely on the
  // blank surface, and a callback is told about that directly instead of the
  // effect needing to depend on a branch decided further down the component.
  const dockRef = useCallback((node: HTMLDivElement | null) => {
    dockObserver.current?.disconnect()
    if (!node) {
      setDockHeight(0)
      return
    }
    const report = () => setDockHeight(node.getBoundingClientRect().height)
    report()
    const observer = new ResizeObserver(report)
    observer.observe(node)
    dockObserver.current = observer
  }, [])
  const seedDraft = useCallback((value: string) => {
    setDraft(value)
    setSeed((previous) => ({ text: value, n: previous.n + 1 }))
  }, [])
  const [fieldHeight, setFieldHeight] = useState(20)
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
  // Where the transcript draws its compaction rules. Interruptions and messages
  // share ONE sequence space, so the boundary is simply the first message whose
  // sequence is past the compaction's.
  //
  // A compaction with nothing after it yet draws NOTHING, and that is the point:
  // the line says "what is above me is gone from the model's context", which is
  // a claim about two sides. Drawing it under the newest message would put a
  // boundary below the whole conversation and read as if the chat had ended.
  const compactionBefore = useMemo(() => {
    const marks: Record<number, string> = {}
    for (const interruption of activity.interruptions) {
      if (interruption.kind !== 'compaction') continue
      const next = ledger.messages.find((m) => m.sequence > interruption.seq)
      if (next) marks[next.sequence] = interruption.detail || 'auto'
    }
    return marks
  }, [activity.interruptions, ledger.messages])
  // The provider's stop reason occupies the BAR, so the transcript must not also
  // render it as a row: it is one sentence, and saying it twice reads as the
  // provider having stopped twice.
  const halted = haltedBy(ledger.messages)

  const updateDraft = (value: string) => {
    setDraft(value)
    setComposerError('')
    slash.noteDraft(value)
  }

  // `text` overrides the draft state for a surface that HOLDS its own text. The
  // document is a contenteditable, so the character that triggered the submit may
  // not have reached React yet — reading state there can enqueue the prompt one
  // keystroke short, or empty. The element is the authority; state is the mirror.
  const enqueueDraft = (text?: string) => {
    const result = prompts.enqueue(text ?? draft)
    if (!result.ok) {
      setComposerError(result.error)
      return
    }
    seedDraft('')
    setComposerError('')
    slash.reset()
  }

  const editPrompt = (item: PromptQueueItem) => {
    prompts.remove(item.clientRequestId)
    seedDraft(item.text)
    setComposerError('')
  }

  const selectSlashItem = (item: SlashCatalogItem) => {
    seedDraft(slash.accept(item))
  }

  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>, readMarkdown: () => string) => {
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
      // The BOX's text, not the draft state. See ChatMarkdownEditor.
      enqueueDraft(readMarkdown())
    }
  }

  // A chat with nothing in it is a DIFFERENT SURFACE, not an empty transcript
  // with a message box under it: the first thing it asks for is a description of
  // the change, which is writing, so it gets the pane at writing size. It stops
  // being one the instant anything is in flight — a queued prompt is already the
  // beginning of a conversation, and so is a failed load worth retrying.
  const nothingYet = ledger.messages.length === 0 && queue.length === 0
  const blank = !ledger.error && nothingYet
  // WHICH SURFACE IS NOT KNOWN UNTIL THE FIRST PAGE LANDS, and guessing shows the
  // wrong one: an empty ledger that has not answered yet is indistinguishable
  // from a chat with history, so picking either paints a composer the reader then
  // watches be replaced. The transcript's own loading state stands alone until
  // the answer arrives — one transition instead of two.
  const settling = ledger.loading && nothingYet

  const selectionCluster = (
    <SelectionCluster
      wsId={wsId}
      chatId={chatId}
      provider={provider}
      providers={providers}
      model={model}
      effort={effort}
      presentation={presentation}
      splitEnabled={splitEnabled && provider?.hotswap === true}
      showSwitcher={presentation !== 'terminal' && provider?.hasTerminal !== false}
      handoverBlocked={!provider?.hotswap && working}
      switchDisabled={switchDisabled}
      onSwitchProvider={onSwitchProvider}
      onSelectionChange={onSelectionChange}
      onSelectPresentation={onSelectPresentation}
    />
  )

  const transcript = (
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
      compactionBefore={compactionBefore}
      suppressSequence={halted?.sequence}
    />
  )

  if (settling) {
    return (
      <section className="agent-chat chat" aria-label="Agent chat">
        {transcript}
      </section>
    )
  }

  if (blank) {
    return (
      <section className="agent-chat chat" aria-label="Agent chat">
        <AgentEmptyDocument
          draft={seed.text}
          draftSeed={seed.n}
          onDraftChange={updateDraft}
          onSubmit={enqueueDraft}
          onKeyDown={handleKeyDown}
          controls={selectionCluster}
          working={working}
          canStop={live}
          onStop={() => void stopChat(wsId, chatId)}
        />
        {composerError && (
          <p className="meta" role="alert">
            {composerError}
          </p>
        )}
      </section>
    )
  }

  return (
    <section
      className="agent-chat chat"
      aria-label="Agent chat"
      style={{ '--agent-dock-h': `${Math.round(dockHeight)}px` } as React.CSSProperties}
    >
      {transcript}

      <div ref={dockRef} className="dock">
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
          draftSeed={seed.n}
          seedText={seed.text}
        />
        <ProviderBar
          wsId={wsId}
          chatId={chatId}
          provider={provider}
          providers={providers}
          onSwitchProvider={onSwitchProvider}
          switchDisabled={switchDisabled}
          model={model}
          effort={effort}
          telemetry={telemetry}
          presentation={presentation}
          splitEnabled={splitEnabled && provider?.hotswap === true}
          compacting={compacting}
          working={working}
          queued={queue.length}
          onCompact={() => void compactChat(wsId, chatId)}
          onSelectionChange={onSelectionChange}
          onSelectPresentation={onSelectPresentation}
          showSwitcher={presentation !== 'terminal' && provider?.hasTerminal !== false}
          handoverBlocked={!provider?.hotswap && working}
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
