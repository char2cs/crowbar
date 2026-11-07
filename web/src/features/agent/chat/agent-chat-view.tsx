import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { CSSProperties, KeyboardEvent, ReactNode } from 'react'
import { report } from '@/lib/errors/report'
import { markEnd, markStart } from '@/lib/perf/instrumentation'
import { DndScope } from '@/features/agent/chat/dnd-scope'
import { compactChat, resumeChat } from '@/features/agent/api/agent-chat-commands'
import { toastSpawnFailure } from '@/features/agent/lib/spawn-error'
import { declaredModel } from '@/features/agent/lib/staged-selection'
import { stopTurn } from '@/features/agent/stores/chat-outbox'
import type { RowActions } from '@/features/agent/transcript/rows/transcript-row'
import type { AgentTerminalAnswer } from '@/features/agent/api/agent-chat-reads'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { SubagentShelf } from '@/features/agent/activity/subagent-shelf'
import { TodoStrip } from '@/features/agent/activity/todo-strip'
import { AgentComposer } from '@/features/agent/composer/agent-composer'
import { acceptsTyping, resolveComposerState } from '@/features/agent/composer/lib/composer-state'
import {
  ComposerSlashPicker,
  type PickerItem,
} from '@/features/agent/composer/composer-slash-picker'
import { ComposerStashPicker } from '@/features/agent/composer/composer-stash-picker'
import { stashPrompt } from '@/features/agent/composer/lib/prompt-stash-persistence'
import type {
  CaretEdges,
  ComposerShape,
} from '@/features/agent/composer/plate/chat-markdown-editor'
import { ChatMarkdownAssetProvider } from '@/features/agent/composer/plate/attachments/chat-markdown-asset-provider'
import { ProviderBar } from '@/features/agent/controls/provider-bar'
import { SelectionCluster } from '@/features/agent/controls/selection-cluster'
import { AgentTranscript } from '@/features/agent/transcript/agent-transcript'
import type { AgentEmptyDocumentHandle } from '@/features/agent/chat/agent-empty-document'
import { handleComposerKey } from '@/features/agent/chat/lib/composer-keys'
import { useChatSend } from '@/features/agent/chat/use-chat-send'
import { useConfirmedModel } from '@/features/agent/chat/use-confirmed-model'
import { useComposerDraft } from '@/features/agent/chat/use-composer-draft'
import { ChatWorkingLine } from '@/features/agent/chat/chat-working-line'
import { promptTexts, useComposerFacts } from '@/features/agent/chat/use-composer-facts'
import { useDock } from '@/features/agent/chat/use-dock'
import { useTypeToFocus } from '@/features/agent/chat/use-type-to-focus'
import { useChatFeed, useFeedError } from '@/features/agent/hooks/use-chat-feed'
import { useChatTelemetry } from '@/features/agent/hooks/use-chat-entity'
import { useNativeContext } from '@/features/agent/hooks/use-native-context'
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
import { UserFacingError } from '@/lib/errors/user-facing-error'

/** One entry per progressive-blur layer in the dock's dissolve — see composer.css. */
const DISSOLVE_LAYERS = [0, 1, 2, 3, 4, 5, 6]

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
  /** The chat exists, so a prompt can be queued: live, dormant or spawning. */
  canSend: boolean
  /** How the conversation continues (why it is dormant, or from what). */
  sessionNote?: string
  /** The CLI the chat is moving onto is still connecting. */
  connecting?: string
  active: boolean
  visible: boolean
  isActivePane: boolean
  onOpenTerminal: () => void
  /** The daemon found the CLI blocked on a prompt Crowbar cannot answer. */
  terminalWaiting?: boolean
  terminalWaitKind?: string
  terminalWaitAnswers?: AgentTerminalAnswer[]
  /** The overlay header's clickable row, for opaque content under it. */
  headerClearancePx?: number
  /** The header's full dissolve reach, which scrolled text must clear. */
  transcriptHeaderClearancePx?: number
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
  presentation: ChatPresentation
  splitEnabled: boolean
  onSelectPresentation: (next: ChatPresentation) => void
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
  canSend,
  sessionNote,
  connecting,
  active,
  visible,
  isActivePane,
  onOpenTerminal,
  terminalWaiting = false,
  terminalWaitKind,
  terminalWaitAnswers,
  headerClearancePx = 0,
  transcriptHeaderClearancePx = 0,
  onBlankChange,
  blankSignpost,
  provider: effectiveProviderId,
  model,
  effort,
  launchModel = '',
  selectionKnown = true,
  onSelectionChange,
  presentation,
  splitEnabled,
  onSelectPresentation,
}: AgentChatViewProps) {
  useChatFeed(chatId)
  const facts = useComposerFacts(chatId)
  const feedError = useFeedError(chatId)
  // A failed feed keeps its rows on screen but takes no more, so the bar says why.
  const unfollowed =
    facts.status === 'error'
      ? feedError || 'Couldn’t reach this chat. Crowbar keeps trying.'
      : undefined
  const { working, compacting } = facts
  const telemetry = useChatTelemetry(chatId)
  // Read once: the view remounts per chat, and the anchor restores on mount.
  const [initialScrollPosition] = useState(() => getScrollPosition(chatId))
  const dock = useDock()
  const [chatSurfaceEl, setChatSurfaceEl] = useState<HTMLElement | null>(null)
  const emptyDocRef = useRef<AgentEmptyDocumentHandle>(null)
  const confirmedModel = useConfirmedModel(`${chatId}:${providerId}`, live, launchModel)

  useEffect(() => markStart('chat.open'), [])
  const opened = facts.status !== undefined && facts.status !== 'connecting'
  useEffect(() => {
    if (!opened) return
    const raf = requestAnimationFrame(() => markEnd('chat.open'))
    return () => cancelAnimationFrame(raf)
  }, [opened])

  const readPrompts = useCallback(() => promptTexts(chatId), [chatId])
  const history = usePromptHistory(readPrompts)
  const composer = useComposerDraft({
    wsId,
    chatId,
    providerId,
    catalogActive: active && visible,
    history,
  })
  const { load, slash, mention, setError } = composer
  const { reset: resetHistory } = history
  const editPrompt = useCallback(
    (text: string) => {
      load(text)
      setError('')
      resetHistory()
    },
    [load, setError, resetHistory],
  )
  const steer = providers.find((candidate) => candidate.id === providerId)?.promptSteer === true
  const prompts = useChatSend(wsId, chatId, steer, editPrompt)

  // An empty chat is a writing surface, not an empty transcript; it stops being
  // one the moment anything is in flight. Until the feed answers, which surface
  // it is cannot be known, so the transcript's loading state stands alone.
  const nothingYet = facts.empty && prompts.queued === 0
  const blank = nothingYet && facts.status === 'live'
  const settling = nothingYet && facts.status === 'connecting'
  // react-doctor-disable-next-line react-doctor/no-pass-live-state-to-parent -- the pane's signpost reads it
  useEffect(() => onBlankChange?.(blank), [blank, onBlankChange])

  const provider = providers.find((candidate) => candidate.id === providerId)
  const providerLabel = provider?.displayName ?? providerId
  const rowActions = useMemo<RowActions>(
    () => ({
      ...prompts.actions,
      openTerminal: onOpenTerminal,
      newConversation: () =>
        void resumeChat(wsId, chatId).catch((err: unknown) =>
          toastSpawnFailure(err, providerLabel, 'start'),
        ),
    }),
    [prompts.actions, wsId, chatId, onOpenTerminal, providerLabel],
  )
  // The picker shows a staged pick at once; labels and the catalogue stay on the live CLI.
  const effectiveProvider = providers.find((candidate) => candidate.id === effectiveProviderId)
  // The person's pick first, then what the last turn reported running at.
  const effectiveEffort = effort || facts.latestEffort
  const reportedModel = telemetry?.model?.displayName || telemetry?.model?.id || ''
  const terminalWait = terminalWaiting
    ? { kind: terminalWaitKind ?? '', answers: terminalWaitAnswers }
    : undefined
  // The pick wins: the chip says what the next message runs as. Unpicked, the
  // report's display name wins over the last confirmed raw launch id.
  const shownModel =
    declaredModel(effectiveProvider, model) || (reportedModel ? '' : confirmedModel)
  const offeredCompact =
    provider?.compaction && live && !compacting && !onTerminalSurface
      ? () => void compactChat(wsId, chatId)
      : undefined
  useNativeContext({ visible, telemetry, compacting, onCompact: offeredCompact })

  // A chat recording no agent cannot start; it says what would let it.
  const agentNote =
    !selectionKnown || effectiveProviderId
      ? undefined
      : providers.some((p) => p.connected && p.enabled)
        ? 'This chat has no agent yet. Pick one, then send a message to start it.'
        : 'No agent is installed. Add one in Settings → Agents to start this chat.'
  const note = providerId ? (blank ? undefined : sessionNote) : agentNote

  // The one serialization of a prompt: the key handler's read, or the box's on a click.
  const enqueueDraft = (text?: string) => {
    if (agentNote) return composer.setError(agentNote)
    const stagedProvider = effectiveProviderId === providerId ? '' : effectiveProviderId
    const result = prompts.send({
      text: chatPromptToSend(text ?? composer.read()),
      provider: stagedProvider,
      model: selectionKnown ? model : undefined,
      effort: selectionKnown ? effort : undefined,
    })
    if (!result.ok) {
      composer.setError(result.error ?? '')
      return
    }
    // Only the first send has an empty document to slide the dock in from.
    if (blank) dock.noteArrivalOrigin(emptyDocRef.current?.getHandleRect() ?? null)
    composer.clear()
    composer.setError('')
    slash.reset()
    history.reset()
  }

  const handleStop = () => void stopTurn(wsId, chatId)

  // An open @ path picker takes the keys before the skill picker: it completes in place.
  const picker = mention.open ? mention : slash
  const selectSlashItem = (item: PickerItem) =>
    mention.open ? composer.completeMention(mention.accept(item)) : load(slash.accept(item))

  // Esc takes the newest prompt back; its text returns to this chat's composer,
  // or to the chat's stashes once that composer is gone.
  const shownChat = useRef<string | null>(null)
  useEffect(() => {
    shownChat.current = chatId
    return () => {
      shownChat.current = null
    }
  }, [chatId])
  const retract = () => {
    const taken = prompts.retract()
    const asked = chatId
    void taken?.then((text) => {
      if (text === null) return
      if (shownChat.current === asked && composer.putBack(text)) return
      if (!stashPrompt(wsId, asked, text)) {
        report(new UserFacingError('The saved prompts are full or unavailable.'), {
          action: 'Couldn’t keep the prompt you took back',
        })
      }
    })
    return taken !== null
  }

  const handleKeyDown = (
    event: KeyboardEvent<HTMLDivElement>,
    readMarkdown: () => string,
    caret: CaretEdges,
    shape: ComposerShape,
  ) =>
    handleComposerKey(
      {
        slash: picker,
        history,
        canStop: working && live,
        stop: handleStop,
        retract,
        stashOrRestore: composer.stashOrRestore,
        selectSlashItem,
        send: enqueueDraft,
        recall: load,
      },
      event,
      readMarkdown,
      caret,
      shape,
    )

  const composerState = () =>
    resolveComposerState({
      live: canSend,
      submitUnavailable: prompts.unsupported,
      terminalWait,
      compacting,
      asks: facts.asks,
      feedError: unfollowed,
    })
  useTypeToFocus({
    wsId,
    watched: active && visible && isActivePane,
    canType: () => acceptsTyping(composerState()),
    onText: (text) => load(composer.read() + text),
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
      status={facts.status}
      working={working}
      tail={<ChatWorkingLine chatId={chatId} connecting={connecting} />}
      onOpenTerminal={onOpenTerminal}
      promptActions={rowActions}
      dockHeight={dock.dockHeight}
      visible={visible}
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

  // One tree for the blank document and the dock: the composer keeps its node, and
  // its text, when the first send moves it down.
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
          {!blank && transcript}
          {!blank && !settling && (
            <div className="dissolve" aria-hidden="true">
              {DISSOLVE_LAYERS.map((_, i) => (
                <div key={i} className="dissolve-layer" />
              ))}
            </div>
          )}
          {blank && stashPicker && <div className="blank-stash-picker">{stashPicker}</div>}
          {!settling && (
            <div ref={blank ? undefined : dock.dockRef} className={blank ? 'docframe' : 'dock'}>
              {!blank && <TodoStrip plan={facts.plan} />}
              {!blank && <SubagentShelf chatId={chatId} />}
              {!blank && stashPicker}
              {!blank && (mention.open || slash.open) && (
                <ComposerSlashPicker
                  state={mention.open ? undefined : slash.state}
                  label={mention.open ? 'Files' : 'Skills'}
                  items={picker.items}
                  selected={picker.selected}
                  onSelect={selectSlashItem}
                />
              )}
              {note && (
                <p className="meta composer-note" data-testid="agent-session-note">
                  {note}
                </p>
              )}
              <AgentComposer
                wsId={wsId}
                chatId={chatId}
                placement={blank ? 'document' : 'dock'}
                editorRef={composer.editorRef}
                asks={facts.asks}
                providerLabel={providerLabel}
                permissionLevels={provider?.permissionLevels}
                live={canSend}
                working={working}
                compacting={compacting}
                sending={prompts.sending}
                submitUnavailable={prompts.unsupported}
                terminalWait={terminalWait}
                feedError={unfollowed}
                canStop={live}
                slashOpen={picker.open}
                onShapeChange={composer.noteShape}
                onKeyDown={handleKeyDown}
                onSend={() => enqueueDraft()}
                onStop={handleStop}
                onOpenTerminal={onOpenTerminal}
                takeoverContainer={chatSurfaceEl}
                controls={<SelectionCluster {...selection} model={shownModel} />}
                banner={blankSignpost}
                headerClearancePx={headerClearancePx}
                documentRef={emptyDocRef}
              />
              {!blank && (
                <ProviderBar
                  {...selection}
                  model={shownModel}
                  telemetry={telemetry}
                  queued={prompts.waiting}
                  onCompact={offeredCompact}
                />
              )}
              {composer.error && (
                <p className="meta composer-note" role="alert">
                  {composer.error}
                </p>
              )}
            </div>
          )}
        </section>
      </ChatMarkdownAssetProvider>
    </DndScope>
  )
}
