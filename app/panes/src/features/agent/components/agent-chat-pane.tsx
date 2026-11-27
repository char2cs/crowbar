import { useCallback, useEffect, useRef, useState } from 'react'
import {
  effectiveSelection,
  stageSelection as stageSelectionFrom,
  type StagedSelection,
} from '@/features/agent/lib/staged-selection'
import { useStore } from 'zustand'
import { isChatRecalled } from '@/features/hosts/lib/host-resolve-cached'
import { TrashIcon } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { ComposerSignpost } from '@/features/agent/composer/composer-signpost'
import { useChatField } from '@/features/agent/hooks/use-chat-entity'
import { useChatListReady } from '@/features/agent/hooks/use-chat-providers'
import { discardUnsent, holdForTerminal } from '@/features/agent/stores/chat-outbox'
import { useChatStore } from '@/features/agent/stores/chat-store'
import { selectRunnerChat, useSelect } from '@/core/entities/select'
import { deliveryPending as promptOnItsWay } from '@/features/agent/stores/lib/outbox'
import { switchProvider } from '@/features/agent/api/agent-chat-commands'
import { useAnswerTerminalWait } from '@/features/agent/hooks/use-answer-terminal-wait'
import { useEffectiveChordMap } from '@/features/keymaps/hooks/use-effective-keymap'
import { AGENT_CYCLE_PROVIDER, AGENT_TOGGLE_VIEW_MODE } from '@/features/keymaps/registry'
import { useZoomStore } from '@/features/window/stores/zoom-store'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { toastSpawnFailure } from '@/features/agent/lib/spawn-error'
import { usePaneAttachment } from '@/features/agent/components/use-pane-attachment'
import { usePaneProviders } from '@/features/agent/hooks/use-pane-providers'
import {
  SPLIT_MIN_HALF_PX,
  SPLIT_MIN_STACKED_PX,
  useChatPresentation,
  useTerminalGridSlack,
} from '@/features/agent/hooks/use-chat-presentation'
import { PaneSash } from '@/features/panes/components/pane-sash'
import { cn } from '@/lib/utils'
import {
  AgentReturnToChatNotice,
  AgentTerminalWaitBanner,
} from '@/features/agent/components/agent-terminal-wait-banner'
import { isHotswap, useTerminalEscort } from '@/features/agent/components/use-terminal-escort'
import { useWindowChord } from '@/features/agent/components/use-window-chord'
import { AgentChatView } from '@/features/agent/chat/agent-chat-view'
import { AgentTerminalSurface } from '@/features/agent/terminal/agent-terminal-surface'
import type { TerminalFocusHandle } from '@/features/terminal/components/terminal'
import { CHROME_ROW_HEIGHT_PX } from '@/lib/chrome-metrics'
import { IS_MOBILE } from '@/utils/platform'

export interface AgentChatPaneProps {
  chatId: string
  runnerId: string
  wsId: string
  paneId: string
  isActivePane: boolean
  isVisible: boolean
  belowOverlayHeader?: boolean
}

const HEADER_ROW_HEIGHT_PX = CHROME_ROW_HEIGHT_PX
const CHAT_BLUR_ZONE_PX = HEADER_ROW_HEIGHT_PX + 56

// react-doctor-disable-next-line no-giant-component -- one live PTY-backed chat whose resize, focus and surface state are coupled
export function AgentChatPaneBody({
  chatId,
  runnerId,
  wsId,
  hostId,
  paneId,
  isActivePane,
  isVisible,
  belowOverlayHeader = false,
}: AgentChatPaneProps & {
  hostId: string
}) {
  const headerClearancePx = belowOverlayHeader ? HEADER_ROW_HEIGHT_PX + 8 : 8
  const transcriptHeaderClearancePx = belowOverlayHeader ? CHAT_BLUR_ZONE_PX : 0

  const [opened, setOpened] = useState(isVisible)
  if (isVisible && !opened) setOpened(true)

  const runnerChatId = useSelect((s) => (runnerId ? selectRunnerChat(s, runnerId) : ''))
  const shownChatId = runnerChatId || chatId

  const listSeeded = useChatListReady()
  const recorded = useStore(windowPaneStore, (s) => s.panes[paneId]?.workspaceId != null)
  const placed = recorded || isChatRecalled(chatId)
  const activeProviderId = useChatField(shownChatId, (c) => c?.activeProviderId ?? '')
  const onTerminalSurface = useChatField(shownChatId, (c) => c?.surface === 'terminal')
  const working = useChatField(shownChatId, (c) => c?.working ?? false)
  const chatModel = useChatField(shownChatId, (c) => c?.model ?? '')
  const chatEffort = useChatField(shownChatId, (c) => c?.effort ?? '')
  const launchModel = useChatField(shownChatId, (c) => c?.launchModel ?? '')
  const providers = usePaneProviders(hostId)
  const heldView = onTerminalSurface ? 'terminal' : 'chat'
  const hotswap = isHotswap(providers, activeProviderId).hotswap
  const heldSurface = hotswap ? null : heldView
  const recordedSurface = useChatField(shownChatId, (c) => (hotswap ? (c?.surface ?? null) : null))

  const waitKind = useChatField(shownChatId, (c) => c?.terminalWait?.kind)
  const waiting = waitKind !== undefined
  const waitAnswers = useChatField(shownChatId, (c) => c?.terminalWait?.answers)
  const answerTerminalWait = useAnswerTerminalWait(wsId, shownChatId)

  const columnRef = useRef<HTMLDivElement>(null)
  const splitContainerRef = useRef<HTMLDivElement>(null)
  const {
    presentation,
    setPresentation,
    splitEnabled,
    splitting,
    returnOffered,
    setReturnOffered,
    splitFocus,
    setSplitFocus,
    splitSizes,
    setSplitSizes,
    splitStacked,
  } = useChatPresentation(shownChatId, splitContainerRef, heldSurface, recordedSurface)
  const terminalOnly = presentation === 'terminal' && !splitting
  useEffect(
    () => (terminalOnly ? holdForTerminal(shownChatId) : undefined),
    [terminalOnly, shownChatId],
  )
  const queuedPromptCount = useChatStore((s) => s.outbox[shownChatId]?.length ?? 0)
  const cancelablePromptCount = useChatStore(
    (s) => s.outbox[shownChatId]?.filter((item) => item.state !== 'sending').length ?? 0,
  )
  const deliveryPending = useChatStore((s) =>
    promptOnItsWay(s.outbox[shownChatId], s.feeds[shownChatId]?.facts),
  )
  const [chatBlank, setChatBlank] = useState(true)
  const chatZoom = useZoomStore.use.zoom()
  const providerName = providers.find((p) => p.id === activeProviderId)?.displayName || 'the agent'
  const { known, liveRunnerId, attachment, revival, connecting, canSend, offline, startSession } =
    usePaneAttachment({
      wsId,
      hostId,
      chatId: shownChatId,
      providerName,
      presentation,
      promptReplacing: deliveryPending,
    })

  const waitingBannerShown = waiting && chatBlank
  const revivingBannerShown =
    presentation !== 'terminal' && chatBlank && attachment.state === 'reviving' && !deliveryPending

  const rootRef = useRef<HTMLDivElement>(null)
  const terminalApiRef = useRef<TerminalFocusHandle | null>(null)
  const chatSurfaceRef = useRef<HTMLDivElement>(null)
  const terminalSurfaceRef = useRef<HTMLDivElement>(null)

  const gridSlack = useTerminalGridSlack(
    columnRef,
    attachment.state === 'attached' && presentation === 'terminal',
  )

  useEffect(() => {
    if (!known || shownChatId !== chatId) return
    if (liveRunnerId === runnerId) return
    windowPaneStore.getState().paneActions.setPaneRunner(paneId, liveRunnerId || null)
  }, [paneId, known, chatId, runnerId, shownChatId, liveRunnerId])

  const [stagedSelection, setStagedSelection] = useState<StagedSelection | null>(null)
  const liveSelection = { providerId: activeProviderId, model: chatModel, effort: chatEffort }
  const {
    providerId: effectiveProviderId,
    model: effectiveModel,
    effort: effectiveEffort,
  } = effectiveSelection(stagedSelection, liveSelection)

  const stageSelection = useCallback(
    (providerId: string, model: string, effort: string) => {
      setStagedSelection(
        stageSelectionFrom(
          { providerId: activeProviderId, model: chatModel, effort: chatEffort },
          { providerId, model, effort },
        ),
      )
    },
    [activeProviderId, chatModel, chatEffort],
  )

  const escort = useTerminalEscort({
    wsId,
    chatId: shownChatId,
    providers,
    activeProviderId,
    onTerminalSurface,
    recordedSurface,
    waiting,
    isVisible,
    presentation,
    setPresentation,
    splitting,
    setSplitFocus,
    setReturnOffered,
    terminalApiRef,
    dormant: attachment.state === 'idle',
    offline,
    startSession,
  })

  const [switchInFlight, setSwitchInFlight] = useState(false)
  const switchBlocked =
    working || switchInFlight || deliveryPending || attachment.state === 'reviving'
  const handleSwitch = async (providerId: string): Promise<boolean> => {
    if (switchBlocked) return false
    const name = providers.find((p) => p.id === providerId)?.displayName ?? providerId
    setSwitchInFlight(true)
    try {
      const surface = presentation === 'split' ? undefined : presentation
      await switchProvider(wsId, shownChatId, providerId, surface)
      if (presentation === 'terminal') void escort.enterTerminal(providerId)
      return true
    } catch (err: unknown) {
      toastSpawnFailure(err, name, 'switch to')
      return false
    } finally {
      setSwitchInFlight(false)
    }
  }

  const chords = useEffectiveChordMap()
  const watched = isActivePane && isVisible
  useWindowChord(chords[AGENT_CYCLE_PROVIDER], watched, wsId, () => {
    const enabled = providers.filter((p) => p.enabled)
    if (enabled.length < 2) return
    const i = enabled.findIndex((p) => p.id === activeProviderId)
    const next = enabled[(i + 1) % enabled.length]
    if (!next || next.id === activeProviderId) return
    void handleSwitch(next.id)
  })
  useWindowChord(chords[AGENT_TOGGLE_VIEW_MODE], watched, wsId, () =>
    escort.chooseSurface(presentation === 'terminal' ? 'chat' : 'terminal'),
  )

  const blankSignpost = waitingBannerShown ? (
    <AgentTerminalWaitBanner
      kind={waitKind ?? ''}
      providerLabel={providers.find((p) => p.id === activeProviderId)?.displayName ?? ''}
      onOpenTerminal={escort.openTerminalFromBanner}
      answers={waitAnswers}
      onAnswer={answerTerminalWait}
    />
  ) : revivingBannerShown && revival ? (
    <div data-testid="agent-reviving-banner">
      <ComposerSignpost reason="reviving" message={revival.message} onOpenTerminal={() => {}} />
    </div>
  ) : undefined

  const focusTerminalFromEmptySpace = (e: React.MouseEvent) => {
    if (attachment.state !== 'attached' || presentation !== 'terminal') return
    if (e.target !== rootRef.current && e.target !== columnRef.current) return
    e.preventDefault()
    terminalApiRef.current?.focus()
  }

  const focusTerminalFromSplitEmptySpace = (e: React.MouseEvent) => {
    if (attachment.state !== 'attached') return
    if (e.target !== terminalSurfaceRef.current) return
    e.preventDefault()
    setSplitFocus('terminal')
    terminalApiRef.current?.focus()
  }

  return (
    <div
      ref={rootRef}
      role="presentation"
      onMouseDown={focusTerminalFromEmptySpace}
      className="flex h-full w-full flex-col"
    >
      <div
        ref={columnRef}
        data-touch={IS_MOBILE || undefined}
        className={cn(
          'agent-chat-pane',
          'mx-auto flex min-h-0 w-full flex-1 flex-col',
          splitting || presentation === 'chat' ? 'max-w-none' : 'max-w-4xl',
          splitting || presentation !== 'chat' ? 'px-4 pt-4' : 'pl-4 data-touch:pl-0',
        )}
      >
        <div
          ref={splitContainerRef}
          className={cn(
            'relative min-h-0 flex-1',
            splitting && (splitStacked ? 'flex flex-col' : 'flex flex-row'),
          )}
        >
          <div
            ref={chatSurfaceRef}
            data-testid="agent-chat-surface"
            data-surface-focused={splitting ? String(splitFocus === 'chat') : undefined}
            onFocusCapture={splitting ? () => setSplitFocus('chat') : undefined}
            className={cn(
              splitting
                ? 'relative min-h-0 min-w-0 shrink grow-0'
                : cn('h-full', presentation !== 'chat' && 'hidden'),
            )}
            style={{
              zoom: chatZoom,
              ...(splitting ? { flexBasis: `${splitSizes[0]}%` } : undefined),
            }}
          >
            {opened && (
              <AgentChatView
                key={`${wsId}:${shownChatId}`}
                wsId={wsId}
                chatId={shownChatId}
                providerId={activeProviderId}
                providers={providers}
                switchDisabled={switchBlocked}
                live={attachment.state === 'attached' || deliveryPending}
                canSend={canSend}
                offline={offline}
                connecting={revivingBannerShown ? undefined : connecting}
                active={presentation === 'chat' || splitting}
                visible={isVisible && (known || (!listSeeded && placed))}
                isActivePane={isActivePane}
                onOpenTerminal={escort.openTerminal}
                terminalWaiting={waiting}
                terminalWaitKind={waitKind ?? ''}
                terminalWaitAnswers={waitAnswers}
                headerClearancePx={headerClearancePx}
                transcriptHeaderClearancePx={transcriptHeaderClearancePx}
                blankSignpost={blankSignpost}
                presentation={presentation}
                splitEnabled={splitEnabled}
                onSelectPresentation={escort.chooseSurface}
                onTerminalSurface={onTerminalSurface}
                provider={effectiveProviderId}
                model={effectiveModel}
                effort={effectiveEffort}
                launchModel={launchModel}
                selectionKnown={known}
                onSelectionChange={stageSelection}
                onBlankChange={setChatBlank}
              />
            )}
          </div>

          {splitting && (
            <PaneSash
              direction={splitStacked ? 'vertical' : 'horizontal'}
              sizes={splitSizes}
              containerRef={splitContainerRef}
              firstPaneRef={chatSurfaceRef}
              secondPaneRef={terminalSurfaceRef}
              onResizeCommit={setSplitSizes}
              minPx={splitStacked ? SPLIT_MIN_STACKED_PX : SPLIT_MIN_HALF_PX}
            />
          )}

          <AgentTerminalSurface
            ref={terminalSurfaceRef}
            wsId={wsId}
            chatId={shownChatId}
            paneId={paneId}
            attachment={attachment}
            presentation={presentation}
            splitting={splitting}
            focused={splitFocus === 'terminal'}
            basis={splitSizes[1]}
            isActivePane={isActivePane}
            isVisible={isVisible}
            gridSlack={gridSlack}
            providers={providers}
            activeProviderId={activeProviderId}
            switchDisabled={switchBlocked}
            splitEnabled={splitEnabled}
            working={working}
            onSwitchProvider={handleSwitch}
            onSelectPresentation={escort.chooseSurface}
            onTakeFocus={() => setSplitFocus('terminal')}
            onDeadSpaceMouseDown={focusTerminalFromSplitEmptySpace}
            terminalRef={terminalApiRef}
            onStartSession={startSession}
            offline={offline}
            headerClearancePx={
              belowOverlayHeader && !(splitting && splitStacked) ? HEADER_ROW_HEIGHT_PX : 0
            }
          />
        </div>

        {presentation === 'terminal' && returnOffered && (
          <AgentReturnToChatNotice
            onReturn={() => escort.chooseSurface('chat')}
            onDismiss={() => setReturnOffered(false)}
          />
        )}

        {presentation === 'terminal' && queuedPromptCount > 0 && (
          <div className="flex items-center justify-between gap-3 border-t py-2 text-muted-foreground text-xs">
            <span>
              {queuedPromptCount} {queuedPromptCount === 1 ? 'prompt' : 'prompts'} pending in Chat
            </span>
            <div className="flex items-center gap-1">
              <Button size="xs" variant="ghost" onClick={() => escort.chooseSurface('chat')}>
                Return to Chat
              </Button>
              {cancelablePromptCount > 0 && (
                <Button
                  size="icon-xs"
                  variant="ghost"
                  aria-label="Cancel unsent prompts"
                  tooltip="Cancel unsent prompts"
                  onClick={() => discardUnsent(shownChatId)}
                >
                  <TrashIcon />
                </Button>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
