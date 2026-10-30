import { useEffect, useImperativeHandle, useRef, useState } from 'react'
import type { CSSProperties, KeyboardEvent, ReactNode, Ref } from 'react'
import { markEnd, markStart } from '@/lib/perf/instrumentation'
import { DndScope } from '@/features/agent/chat/dnd-scope'
import {
  compactChat,
  stopChat,
  type AgentPromptResult,
} from '@/features/agent/api/agent-chat-commands'
import type { SlashCatalogItem } from '@/features/agent/api/agent-chat-reads'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import type { PromptQueueItem } from '@/features/agent/lib/prompt-queue-persistence'
import { SubagentShelf } from '@/features/agent/activity/subagent-shelf'
import { AgentComposer } from '@/features/agent/composer/agent-composer'
import {
  acceptsTyping,
  resolveComposerState,
  type ComposerRevival,
} from '@/features/agent/composer/lib/composer-state'
import { ComposerSlashPicker } from '@/features/agent/composer/composer-slash-picker'
import { ComposerStashPicker } from '@/features/agent/composer/composer-stash-picker'
import type { CaretEdges } from '@/features/agent/composer/plate/chat-markdown-editor'
import { ChatMarkdownAssetProvider } from '@/features/agent/composer/plate/attachments/chat-markdown-asset-provider'
import { ProviderBar } from '@/features/agent/controls/provider-bar'
import { SelectionCluster } from '@/features/agent/controls/selection-cluster'
import { AgentTranscript } from '@/features/agent/transcript/agent-transcript'
import {
  AgentEmptyDocument,
  type AgentEmptyDocumentHandle,
} from '@/features/agent/chat/agent-empty-document'
import { handleComposerKey } from '@/features/agent/chat/lib/composer-keys'
import { useChatSend } from '@/features/agent/chat/use-chat-send'
import { useConfirmedModel } from '@/features/agent/chat/use-confirmed-model'
import { useComposerDraft } from '@/features/agent/chat/use-composer-draft'
import { useComposerFacts } from '@/features/agent/chat/use-composer-facts'
import { useDock } from '@/features/agent/chat/use-dock'
import { useTypeToFocus } from '@/features/agent/chat/use-type-to-focus'
import { useChatFeed } from '@/features/agent/hooks/use-chat-feed'
import { useChatTelemetry, limitResetsAt } from '@/features/agent/hooks/use-chat-entity'
import {
  getScrollPosition,
  setScrollPosition,
} from '@/features/agent/hooks/lib/transcript-scroll-positions'
import { usePromptHistory } from '@/features/agent/hooks/use-prompt-history'
import type { ChatPresentation } from '@/features/settings/lib/chat-presentation'

import '@/features/agent/styles/composer.css'
import '@/features/agent/styles/transcript.css'
import '@/features/agent/styles/activity.css'
import { chatPromptToSend } from '@/features/agent/composer/plate/chat-prompt-send'

/** One entry per progressive-blur layer in the dock's dissolve — see composer.css. */
const DISSOLVE_LAYERS = [0, 1, 2, 3, 4, 5, 6]

export interface AgentChatViewHandle {
  cancelUnsentPrompts: () => void
}

export interface AgentChatViewProps {
  wsId: string
  chatId: string
  /** The chat's live provider: what its CLI is now, never a staged pick. */
  providerId: string
  providers: AgentProvider[]
  /** On the provider's own terminal the daemon refuses a Crowbar compaction. */
  onTerminalSurface?: boolean
  /** A switch is already running, or the pane is mid-delivery. */
  switchDisabled?: boolean
  live: boolean
  /** A send may be dispatched now: live, or dormant and revived by the send. */
  canSend: boolean
  revival?: ComposerRevival
  /** How the conversation continues (why it is dormant, or from what). */
  sessionNote?: string
  active: boolean
  visible: boolean
  /** The chat list has not landed, so the surrounding controls are not final. */
  awaitingList?: boolean
  isActivePane: boolean
  onOpenTerminal: () => void
  /** The daemon found the CLI blocked on a prompt Crowbar cannot answer. */
  terminalWaiting?: boolean
  terminalWaitKind?: string
  /** The overlay header's clickable row, for opaque content under it. */
  headerClearancePx?: number
  /** The header's full dissolve reach, which scrolled text must clear. */
  transcriptHeaderClearancePx?: number
  onPromptSpawned?: (result: AgentPromptResult) => void | Promise<void>
  onPromptDispatchStart?: () => void
  onPromptDispatchSettled?: () => void
  onQueueCountChange?: (count: number) => void
  onCancelableQueueCountChange?: (count: number) => void
  onDeliveryPendingChange?: (pending: boolean) => void
  /** Nothing has been said yet: the empty document stands in for the dock. */
  onBlankChange?: (blank: boolean) => void
  /** The pane's own signpost for a blank chat, in the empty document's control slot. */
  blankSignpost?: ReactNode
  /** The effective provider/model/effort: the chat's, or a staged pick over it. */
  provider: string
  model: string
  effort: string
  /** What the live runner spawned as; '' on a dormant chat. */
  launchModel?: string
  /** False until the chat list lands: '' would then read as a deliberate clear. */
  selectionKnown?: boolean
  onSelectionChange: (provider: string, model: string, effort: string) => void
  onSelectionCommitted?: (model: string, effort: string) => void
  presentation: ChatPresentation
  splitEnabled: boolean
  onSelectPresentation: (next: ChatPresentation) => void
  ref?: Ref<AgentChatViewHandle>
}

/** The chat surface: a transcript and one bar under it, wired from the feed. */
// react-doctor-disable-next-line react-doctor/no-giant-component -- the wiring between the view's hooks and parts is its one job
export function AgentChatView({
  wsId,
  chatId,
  providerId,
  providers,
  onTerminalSurface = false,
  switchDisabled,
  live,
  revival,
  canSend,
  sessionNote,
  active,
  visible,
  awaitingList = false,
  isActivePane,
  onOpenTerminal,
  terminalWaiting = false,
  terminalWaitKind,
  headerClearancePx = 0,
  transcriptHeaderClearancePx = 0,
  onPromptSpawned,
  onPromptDispatchStart,
  onPromptDispatchSettled,
  onQueueCountChange,
  onCancelableQueueCountChange,
  onDeliveryPendingChange,
  onBlankChange,
  blankSignpost,
  provider: effectiveProviderId,
  model,
  effort,
  launchModel = '',
  selectionKnown = true,
  onSelectionChange,
  onSelectionCommitted,
  presentation,
  splitEnabled,
  onSelectPresentation,
  ref,
}: AgentChatViewProps) {
  useChatFeed(chatId)
  const facts = useComposerFacts(chatId)
  const { working, compacting } = facts
  const telemetry = useChatTelemetry(wsId, chatId, visible)
  // Read once: the view remounts per chat, and the anchor restores on mount.
  const [initialScrollPosition] = useState(() => getScrollPosition(chatId))
  const dock = useDock()
  const [chatSurfaceEl, setChatSurfaceEl] = useState<HTMLElement | null>(null)
  const emptyDocRef = useRef<AgentEmptyDocumentHandle>(null)
  const [fieldHeight, setFieldHeight] = useState(20)
  const [submitUnavailable, setSubmitUnavailable] = useState(false)
  // The key remounts on a chat change, not on a provider change alone.
  // react-doctor-disable-next-line react-doctor/no-adjust-state-on-prop-change -- a provider switch must re-offer the input
  useEffect(() => setSubmitUnavailable(false), [chatId, providerId])
  const confirmedModel = useConfirmedModel(`${chatId}:${providerId}`, live, launchModel)

  const prompts = useChatSend({
    wsId,
    chatId,
    facts,
    steerable: providers.find((candidate) => candidate.id === providerId)?.promptSteer === true,
    live,
    canSend,
    active,
    visible,
    terminalWaiting,
    onPromptSpawned,
    onPromptDispatchStart,
    onPromptDispatchSettled,
    onSubmitUnavailable: () => setSubmitUnavailable(true),
    onSelectionCommitted,
    onQueueCountChange,
    onCancelableQueueCountChange,
    onDeliveryPendingChange,
  })

  useEffect(() => markStart('chat.open'), [])
  const opened = facts.status !== undefined && facts.status !== 'connecting'
  useEffect(() => {
    if (!opened) return
    const raf = requestAnimationFrame(() => markEnd('chat.open'))
    return () => cancelAnimationFrame(raf)
  }, [opened])

  const history = usePromptHistory(facts.userTexts)
  const composer = useComposerDraft({
    wsId,
    chatId,
    providerId,
    catalogActive: active && visible,
    history,
  })
  const { draft, seed, seedDraft, slash } = composer

  useImperativeHandle(ref, () => ({ cancelUnsentPrompts: prompts.cancelUnsentPrompts }), [
    prompts.cancelUnsentPrompts,
  ])

  const { queue } = prompts
  // An empty chat is a writing surface, not an empty transcript; it stops being
  // one the moment anything is in flight. Until the feed answers, which surface
  // it is cannot be known, so the transcript's loading state stands alone.
  const nothingYet = facts.empty && queue.length === 0
  const blank = nothingYet && facts.status === 'live'
  const settling = nothingYet && facts.status === 'connecting'
  // react-doctor-disable-next-line react-doctor/no-pass-live-state-to-parent -- the pane's signpost reads it
  useEffect(() => onBlankChange?.(blank), [blank, onBlankChange])

  const provider = providers.find((candidate) => candidate.id === providerId)
  const providerLabel = provider?.displayName ?? providerId
  // The picker shows a staged pick at once; labels and the catalogue stay on the live CLI.
  const effectiveProvider = providers.find((candidate) => candidate.id === effectiveProviderId)
  // The person's pick first, then what the last turn reported running at.
  const effectiveEffort = effort || facts.latestEffort
  const reportedModel = telemetry?.model?.displayName || telemetry?.model?.id || ''
  const terminalWait = terminalWaiting ? { kind: terminalWaitKind ?? '' } : undefined
  const halted = facts.halted

  // `text` is the box's own markdown: the keystroke that sent may not be in state yet.
  const enqueueDraft = (text?: string) => {
    const stagedProvider = effectiveProviderId === providerId ? '' : effectiveProviderId
    const result = prompts.enqueue(
      chatPromptToSend(text ?? draft),
      stagedProvider,
      selectionKnown ? model : undefined,
      selectionKnown ? effort : undefined,
    )
    if (!result.ok) {
      composer.setError(result.error ?? '')
      return
    }
    // Only the first send has an empty document to slide the dock in from.
    if (blank) dock.noteArrivalOrigin(emptyDocRef.current?.getHandleRect() ?? null)
    seedDraft('')
    composer.setError('')
    slash.reset()
    history.reset()
  }

  const editPrompt = (item: PromptQueueItem) => {
    prompts.remove(item.clientRequestId)
    seedDraft(item.text)
    composer.setError('')
    history.reset()
  }

  // Re-seed with the box's text: a stop can make the CLI exit and remount the field.
  const handleStop = () => {
    seedDraft(draft)
    void stopChat(wsId, chatId)
  }

  const selectSlashItem = (item: SlashCatalogItem) => seedDraft(slash.accept(item))

  const handleKeyDown = (
    event: KeyboardEvent<HTMLDivElement>,
    readMarkdown: () => string,
    caret: CaretEdges,
  ) =>
    handleComposerKey(
      {
        slash,
        history,
        canStop: working && live,
        stop: handleStop,
        stashOrRestore: composer.stashOrRestore,
        selectSlashItem,
        send: enqueueDraft,
        recall: seedDraft,
      },
      event,
      readMarkdown,
      caret,
    )

  const composerState = () =>
    resolveComposerState({
      live: canSend,
      revival,
      submitUnavailable,
      terminalWait,
      compacting,
      asks: facts.asks,
      haltedMessage: halted?.text,
      haltedResetsAt: limitResetsAt(telemetry),
    })
  useTypeToFocus({
    wsId,
    watched: active && visible && isActivePane,
    canType: () => acceptsTyping(composerState()),
    onText: (text) => seedDraft(draft + text),
  })

  // The empty document's cluster and the dock's bar offer the same controls.
  const selection = {
    provider: effectiveProvider,
    providers,
    effort: effectiveEffort,
    reportedModel,
    presentation,
    splitEnabled: splitEnabled && provider?.hotswap === true,
    showSwitcher: presentation !== 'terminal' && provider?.hasTerminal !== false,
    handoverBlocked: !provider?.hotswap && working,
    switchDisabled,
    onSelectionChange,
    onSelectPresentation,
  }

  const transcript = (
    <AgentTranscript
      chatId={chatId}
      providers={providers}
      queue={queue}
      status={facts.status}
      working={working}
      since={facts.since}
      compacting={compacting}
      reasoning={facts.reasoning}
      plan={facts.plan}
      asking={facts.asks.length > 0}
      suppressId={halted?.id}
      onOpenTerminal={onOpenTerminal}
      onEditPrompt={editPrompt}
      onCancelPrompt={prompts.remove}
      onRetryPrompt={prompts.retry}
      showTerminalHintFor={
        prompts.showAwaitingTerminalHint ? prompts.awaitingHead?.clientRequestId : undefined
      }
      dockHeight={dock.dockHeight}
      visible={visible}
      holdReveal={awaitingList}
      initialScrollPosition={initialScrollPosition}
      onScrollPositionChange={(position) => setScrollPosition(chatId, position)}
      headerClearancePx={transcriptHeaderClearancePx}
    />
  )

  // Two clearances: scrolled text must clear the header's whole dissolve, the
  // composer and empty document only its clickable row.
  const headerClearanceStyle = {
    '--agent-header-clearance': `${headerClearancePx}px`,
    '--agent-transcript-header-clearance': `${transcriptHeaderClearancePx}px`,
  } as CSSProperties

  const stashPicker = composer.stashPickerOpen && (
    <ComposerStashPicker
      items={composer.stashes}
      onSelect={composer.restoreStash}
      onClose={composer.closeStashPicker}
    />
  )

  if (settling || blank) {
    return (
      <DndScope>
        <ChatMarkdownAssetProvider wsId={wsId} chatId={chatId}>
          <section className="agent-chat chat" aria-label="Agent chat" style={headerClearanceStyle}>
            {settling ? (
              transcript
            ) : (
              <>
                {stashPicker && <div className="blank-stash-picker">{stashPicker}</div>}
                <AgentEmptyDocument
                  ref={emptyDocRef}
                  wsId={wsId}
                  chatId={chatId}
                  draft={seed.text}
                  draftSeed={seed.n}
                  hasText={draft.trim().length > 0}
                  onDraftChange={composer.updateDraft}
                  onSubmit={() => enqueueDraft()}
                  onKeyDown={handleKeyDown}
                  controls={<SelectionCluster {...selection} model={model} />}
                  working={working}
                  canStop={live}
                  sending={prompts.deliveryPending}
                  onStop={handleStop}
                  headerClearancePx={headerClearancePx}
                  banner={blankSignpost}
                />
                {composer.error && (
                  <p className="meta composer-note" role="alert">
                    {composer.error}
                  </p>
                )}
              </>
            )}
          </section>
        </ChatMarkdownAssetProvider>
      </DndScope>
    )
  }

  return (
    <DndScope>
      <ChatMarkdownAssetProvider wsId={wsId} chatId={chatId}>
        <section
          ref={setChatSurfaceEl}
          className="agent-chat chat"
          aria-label="Agent chat"
          style={
            {
              ...headerClearanceStyle,
              '--agent-dock-h': `${Math.round(dock.dockHeight)}px`,
              '--agent-scrollbar-w': `${dock.scrollbarWidth}px`,
            } as CSSProperties
          }
        >
          {transcript}
          <div className="dissolve" aria-hidden="true">
            {DISSOLVE_LAYERS.map((_, i) => (
              <div key={i} className="dissolve-layer" />
            ))}
          </div>
          <div ref={dock.dockRef} className="dock">
            <SubagentShelf subagents={facts.subagents} />
            {stashPicker}
            {slash.open && (
              <ComposerSlashPicker
                state={slash.state}
                items={slash.items}
                selected={slash.selected}
                onSelect={selectSlashItem}
              />
            )}
            {sessionNote && (
              <p className="meta composer-note" data-testid="agent-session-note">
                {sessionNote}
              </p>
            )}
            <AgentComposer
              wsId={wsId}
              chatId={chatId}
              asks={facts.asks}
              providerLabel={providerLabel}
              permissionLevels={provider?.permissionLevels}
              live={canSend}
              revival={revival}
              working={working}
              compacting={compacting}
              sending={prompts.deliveryPending}
              submitUnavailable={submitUnavailable}
              terminalWait={terminalWait}
              haltedMessage={halted?.text}
              haltedResetsAt={limitResetsAt(telemetry)}
              canStop={live}
              draft={draft}
              fieldHeight={fieldHeight}
              slashOpen={slash.open}
              onDraftChange={composer.updateDraft}
              onHeightChange={setFieldHeight}
              onKeyDown={handleKeyDown}
              onSend={() => enqueueDraft()}
              onStop={handleStop}
              onOpenTerminal={onOpenTerminal}
              draftSeed={seed.n}
              seedText={seed.text}
              takeoverContainer={chatSurfaceEl}
            />
            <ProviderBar
              {...selection}
              // The pick wins: this chip says what the next message runs as. The
              // launch report only fills the gap, from the last one confirmed.
              model={model || confirmedModel}
              telemetry={telemetry}
              queued={queue.length}
              onCompact={
                provider?.compaction && live && !compacting && !onTerminalSurface
                  ? () => void compactChat(wsId, chatId)
                  : undefined
              }
            />
            {(composer.error || prompts.persistenceLost) && (
              <p className="meta composer-note" role="alert">
                {composer.error ||
                  'Pending prompts cannot be saved on this device. Keep Crowbar open until they finish.'}
              </p>
            )}
          </div>
        </section>
      </ChatMarkdownAssetProvider>
    </DndScope>
  )
}
