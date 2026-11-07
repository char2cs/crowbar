import { useCallback, useEffect, useRef, useState } from 'react'
import {
  effectiveSelection,
  stageSelection as stageSelectionFrom,
  type StagedSelection,
} from '@/features/agent/lib/staged-selection'
import { useStore } from 'zustand'
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
  /** The chat this pane is pointed at. NOT stable for its life: the pane re-points it. */
  chatId: string
  /** The runner this pane follows, or '' when it has none (a dormant chat). */
  runnerId: string
  wsId: string
  /** The `PaneGroup` this surface renders inside. */
  paneId: string
  isActivePane: boolean
  /** This tab is the visible one in its pane; a hidden tab stays mounted, its chat attached once shown. */
  isVisible: boolean
  /** An overlay chat header owns a clickable row that top-pinned surfaces must clear. */
  belowOverlayHeader?: boolean
}

// PaneTopRow's clickable row; that file's CHAT_BLUR_EXTRA_PX (56) is the
// header dissolve's further reach, which scrolled text must clear too.
const HEADER_ROW_HEIGHT_PX = CHROME_ROW_HEIGHT_PX
const CHAT_BLUR_ZONE_PX = HEADER_ROW_HEIGHT_PX + 56

/** A flat pane showing a RUNNER, not a chat: it follows its runner when the CLI
 *  moves to another chat (/clear, /resume), else adopts whoever is on its chat.
 *  The terminal is keyed by the runner's PTY, so a move never remounts xterm. */
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
  /** The host `wsId` resolved to: the gate mounts this body only once it has. */
  hostId: string
}) {
  // Banners clear the header's row (+8px of air); the transcript its whole dissolve.
  const headerClearancePx = belowOverlayHeader ? HEADER_ROW_HEIGHT_PX + 8 : 8
  const transcriptHeaderClearancePx = belowOverlayHeader ? CHAT_BLUR_ZONE_PX : 0

  // A restored tab opens its chat the first time it is shown, not behind the one in view.
  const [opened, setOpened] = useState(isVisible)
  if (isVisible && !opened) setOpened(true)

  // My runner's chat, '' when it is nowhere; a move shows up as a different id.
  const runnerChatId = useSelect((s) => (runnerId ? selectRunnerChat(s, runnerId) : ''))
  const shownChatId = runnerChatId || chatId

  // A landed list turns `!known` from "not yet" into "not in it".
  const listSeeded = useChatListReady()
  // The pane's recorded workspace vouches for the chat until the list lands.
  const placed = useStore(windowPaneStore, (s) => s.panes[paneId]?.workspaceId != null)
  const activeProviderId = useChatField(shownChatId, (c) => c?.activeProviderId ?? '')
  // On 'terminal' the daemon has no api connection and refuses a Crowbar compaction.
  const onTerminalSurface = useChatField(shownChatId, (c) => c?.surface === 'terminal')
  const working = useChatField(shownChatId, (c) => c?.working ?? false)
  const chatModel = useChatField(shownChatId, (c) => c?.model ?? '')
  const chatEffort = useChatField(shownChatId, (c) => c?.effort ?? '')
  // What the live runner spawned as: the current launch, never the next one's request.
  const launchModel = useChatField(shownChatId, (c) => c?.launchModel ?? '')
  const providers = usePaneProviders(wsId, hostId)
  // Without hot-swap the daemon has one view of this chat, and the pane shows that one.
  const heldView = onTerminalSurface ? 'terminal' : 'chat'
  const hotswap = isHotswap(providers, activeProviderId).hotswap
  const heldSurface = hotswap ? null : heldView
  // With hot-swap the daemon holds both views, and records the one last shown.
  const recordedSurface = useChatField(shownChatId, (c) => (hotswap ? (c?.surface ?? null) : null))

  // A prompt Crowbar cannot answer: undefined is none, '' is unidentified.
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
  // Chat's prompts wait while the Terminal alone shows: one typed into the PTY garbles the TUI.
  const terminalOnly = presentation === 'terminal' && !splitting
  useEffect(
    () => (terminalOnly ? holdForTerminal(shownChatId) : undefined),
    [terminalOnly, shownChatId],
  )
  const queuedPromptCount = useChatStore((s) => s.outbox[shownChatId]?.length ?? 0)
  const cancelablePromptCount = useChatStore(
    (s) => s.outbox[shownChatId]?.filter((item) => item.state !== 'sending').length ?? 0,
  )
  // A send may replace or revive the CLI: the pane is mid-delivery until a turn takes it.
  const deliveryPending = useChatStore((s) =>
    promptOnItsWay(s.outbox[shownChatId], s.feeds[shownChatId]?.facts),
  )
  // Fails open: a stray banner frame beside a composer costs nothing, a missing one does.
  const [chatBlank, setChatBlank] = useState(true)
  // CSS zoom relayouts the chat only; the terminal has its own font zoom.
  const chatZoom = useZoomStore.use.zoom()
  const providerName = providers.find((p) => p.id === activeProviderId)?.displayName || 'the agent'
  const {
    known,
    liveRunnerId,
    attachment,
    revival,
    sessionNote,
    connecting,
    canSend,
    startSession,
  } = usePaneAttachment({
    wsId,
    chatId: shownChatId,
    providerName,
    presentation,
    promptReplacing: deliveryPending,
  })

  // At most one blank-chat signpost: waiting needs a live runner, reviving none.
  const waitingBannerShown = waiting && chatBlank
  const revivingBannerShown =
    presentation !== 'terminal' && chatBlank && attachment.state === 'reviving' && !deliveryPending

  const rootRef = useRef<HTMLDivElement>(null)
  const terminalApiRef = useRef<TerminalFocusHandle | null>(null)
  // The halves PaneSash resizes imperatively during a drag.
  const chatSurfaceRef = useRef<HTMLDivElement>(null)
  const terminalSurfaceRef = useRef<HTMLDivElement>(null)

  const gridSlack = useTerminalGridSlack(
    columnRef,
    attachment.state === 'attached' && presentation === 'terminal',
  )

  // Keep the pane's runner in step with its own chat (following it elsewhere is the stream's).
  useEffect(() => {
    if (!known || shownChatId !== chatId) return
    if (liveRunnerId === runnerId) return
    windowPaneStore.getState().paneActions.setPaneRunner(paneId, liveRunnerId || null)
  }, [paneId, known, chatId, runnerId, shownChatId, liveRunnerId])

  // A pick not sent yet, provider included: it commits with the next prompt.
  const [stagedSelection, setStagedSelection] = useState<StagedSelection | null>(null)
  // Derived at render, so a change made elsewhere wins over a stale pick.
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
    isActivePane,
    isVisible,
    presentation,
    setPresentation,
    splitting,
    setSplitFocus,
    setReturnOffered,
    terminalApiRef,
  })

  // Switch on the chat the runner is in now. Awaitable: a cross-provider model
  // pick is two writes, and the second is legal only once the first landed.
  const [switchInFlight, setSwitchInFlight] = useState(false)
  const switchBlocked =
    working || switchInFlight || deliveryPending || attachment.state === 'reviving'
  const handleSwitch = async (providerId: string): Promise<boolean> => {
    if (switchBlocked) return false
    const name = providers.find((p) => p.id === providerId)?.displayName ?? providerId
    setSwitchInFlight(true)
    try {
      // Split keeps the chat's own surface; a single view starts the new CLI where it is shown.
      const surface = presentation === 'split' ? undefined : presentation
      await switchProvider(wsId, shownChatId, providerId, surface)
      // Only enterTerminal knows whether this provider needs a native view forked.
      if (presentation === 'terminal') escort.enterTerminal(providerId)
      return true
    } catch (err: unknown) {
      toastSpawnFailure(err, name, 'switch to')
      return false
    } finally {
      setSwitchInFlight(false)
    }
  }

  // The chords answer only the pane the user is looking at.
  const chords = useEffectiveChordMap()
  const watched = isActivePane && isVisible
  useWindowChord(chords[AGENT_CYCLE_PROVIDER], watched, wsId, () => {
    const enabled = providers.filter((p) => p.enabled)
    if (enabled.length < 2) return
    // An unknown current provider starts the cycle at the top.
    const i = enabled.findIndex((p) => p.id === activeProviderId)
    const next = enabled[(i + 1) % enabled.length]
    if (!next || next.id === activeProviderId) return
    void handleSwitch(next.id)
  })
  useWindowChord(chords[AGENT_TOGGLE_VIEW_MODE], watched, wsId, () =>
    escort.chooseSurface(presentation === 'terminal' ? 'chat' : 'terminal'),
  )

  // Why a blank chat cannot be typed into, in the empty document's control slot.
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

  // A click on the gutters would blur the terminal; a whitelist, so nothing else matches.
  const focusTerminalFromEmptySpace = (e: React.MouseEvent) => {
    if (attachment.state !== 'attached' || presentation !== 'terminal') return
    if (e.target !== rootRef.current && e.target !== columnRef.current) return
    e.preventDefault()
    terminalApiRef.current?.focus()
  }

  // In split the gutters name no surface; only the terminal half's own strip counts.
  const focusTerminalFromSplitEmptySpace = (e: React.MouseEvent) => {
    if (attachment.state !== 'attached') return
    if (e.target !== terminalSurfaceRef.current) return
    e.preventDefault()
    setSplitFocus('terminal')
    terminalApiRef.current?.focus()
  }

  return (
    // No card and no background: the pane region's own surface shows through.
    <div
      ref={rootRef}
      role="presentation"
      onMouseDown={focusTerminalFromEmptySpace}
      className="flex h-full w-full flex-col"
    >
      {/* One column holds the terminal and its switcher, inset by the same box.
          Its cap resizes the PTY, so a lone terminal keeps all four gutters; chat
          and split drop the cap and keep only the left gutter beside the sidebar. */}
      <div
        ref={columnRef}
        // The phone's sidebar is native: no neighbour to keep the left gutter from.
        data-touch={IS_MOBILE || undefined}
        className={cn(
          // Shared controls hang off this scope: the terminal is not inside `.agent-chat`.
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
          {/* Both stay mounted; `hidden` is the dormancy split gives up. */}
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
                sessionNote={sessionNote}
                connecting={revivingBannerShown ? undefined : connecting}
                // In split the chat is in front of the user, so it is active.
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
                // Until `known`, model/effort are '' fallbacks a send would commit as a clear.
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
