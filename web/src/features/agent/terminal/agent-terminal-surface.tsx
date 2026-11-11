import { createPortal } from 'react-dom'
import type { CSSProperties, MouseEvent, Ref, RefObject } from 'react'
import { Button } from '@/components/ui/button'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { ProviderSwitchDropdown } from '@/features/agent/components/provider-switch-dropdown'
import { useChatHeaderSlotElement } from '@/features/agent/hooks/chat-header-slot'
import { ViewSwitcher } from '@/features/agent/controls/view-switcher'
import type { ChatPresentation } from '@/features/settings/lib/chat-presentation'
import { LazyXtermTerminal } from '@/features/terminal/components/lazy-terminal'
import type { TerminalFocusHandle } from '@/features/terminal/components/terminal'
import { TerminalKeyBar } from '@/features/terminal/components/terminal-key-bar'
import { cn } from '@/lib/utils'
import { IS_MOBILE } from '@/utils/platform'

/** `pending` is not idle: offering Resume there would spawn a second CLI. A null
 *  sessionId is a live runner with no terminal (codex), not dormancy. */
export type TerminalAttachment =
  | { state: 'pending' }
  | { state: 'attached'; sessionId: string | null }
  | { state: 'reviving'; message: string }
  | { state: 'idle'; message?: string }

export interface AgentTerminalSurfaceProps {
  wsId: string
  /** The CLI session is owned by the chat, not the worktree sibling chats share. */
  chatId: string
  /** The pane this chat lives in. */
  paneId: string
  attachment: TerminalAttachment
  presentation: ChatPresentation
  splitting: boolean
  /** This half holds the keyboard. Only meaningful while splitting. */
  focused: boolean
  /** Percentage of the split this half occupies. */
  basis: number
  isActivePane: boolean
  isVisible: boolean
  /** Right inset so the strip ends on the character grid's last column. */
  gridSlack: number
  providers: AgentProvider[]
  activeProviderId: string
  switchDisabled: boolean
  splitEnabled: boolean
  /** A turn is in flight — gates handover back to chat. */
  working?: boolean
  onSwitchProvider: (providerId: string) => void
  onSelectPresentation: (next: ChatPresentation) => void
  onTakeFocus: () => void
  /** A click on this half's dead space, pointed back at the grid. */
  onDeadSpaceMouseDown: (event: MouseEvent<HTMLDivElement>) => void
  /** The terminal's focus handle, for moving the keyboard onto the grid. */
  terminalRef: RefObject<TerminalFocusHandle | null>
  /** Bring the chat's provider back without sending anything. */
  onStartSession: () => void
  /** The chat's host does not answer: a session cannot be started now. */
  offline: boolean
  ref?: Ref<HTMLDivElement>
}

/** A sibling of the chat surface, not a variant: they share only the surface
 *  switcher, the one fact that is the same on both. */
export function AgentTerminalSurface({
  wsId,
  chatId,
  paneId,
  attachment,
  presentation,
  splitting,
  focused,
  basis,
  isActivePane,
  isVisible,
  gridSlack,
  providers,
  activeProviderId,
  switchDisabled,
  splitEnabled,
  working,
  onSwitchProvider,
  onSelectPresentation,
  onTakeFocus,
  onDeadSpaceMouseDown,
  terminalRef,
  onStartSession,
  offline,
  ref,
}: AgentTerminalSurfaceProps) {
  const provider = providers.find((candidate) => candidate.id === activeProviderId)
  const headerSlot = useChatHeaderSlotElement()
  // EXACTLY ONE view switcher exists on a pane. In split the chat's provider
  // bar is on screen and carries it; on the terminal surface it is here.
  const controls = (
    <>
      {presentation === 'terminal' && provider?.hasTerminal !== false && (
        <ViewSwitcher
          presentation={presentation}
          splitEnabled={splitEnabled && provider?.hotswap === true}
          handoverBlocked={!provider?.hotswap && working}
          onSelect={onSelectPresentation}
        />
      )}
      <ProviderSwitchDropdown
        providers={providers}
        currentProviderId={activeProviderId}
        onSwitch={onSwitchProvider}
        disabled={switchDisabled}
        compact={Boolean(headerSlot)}
      />
    </>
  )
  return (
    <div
      ref={ref}
      // The mousedown only handles dead space around the grid, so this is not a control.
      role="presentation"
      data-testid="agent-terminal-surface"
      data-surface-focused={splitting ? String(focused) : undefined}
      onFocusCapture={splitting ? onTakeFocus : undefined}
      onMouseDown={splitting ? onDeadSpaceMouseDown : undefined}
      className={cn(
        // A column, so the status strip never lays out over the terminal.
        'flex flex-col',
        splitting
          ? 'relative min-h-0 min-w-0 shrink grow-0'
          : cn('h-full', presentation === 'terminal' ? '' : 'hidden'),
      )}
      style={splitting ? ({ flexBasis: `${basis}%` } as CSSProperties) : undefined}
    >
      {attachment.state === 'attached' && attachment.sessionId && (
        // No key={sessionId}: runner replacement swaps the PTY imperatively. isActive
        // is focus (xterm grabs the caret), so in split it needs this half to hold
        // the keyboard or composer typing would be stolen.
        <LazyXtermTerminal
          sessionId={attachment.sessionId}
          workspaceId={wsId}
          chatId={chatId}
          paneId={paneId}
          isActive={
            isActivePane && isVisible && (presentation === 'terminal' || (splitting && focused))
          }
          isVisible={isVisible && presentation !== 'chat'}
          attachOnly
          flush
          ref={terminalRef}
        />
      )}

      {/* The keys a phone keyboard lacks, under the grid. */}
      {IS_MOBILE &&
        presentation === 'terminal' &&
        attachment.state === 'attached' &&
        attachment.sessionId && (
          <TerminalKeyBar onSend={(bytes) => terminalRef?.current?.send(bytes)} />
        )}

      {presentation !== 'chat' && attachment.state === 'attached' && !attachment.sessionId && (
        // A live runner with nothing attached (codex): no Resume, nothing to restart.
        <div className="flex h-full w-full flex-col items-center justify-center gap-3 p-6">
          <p className="max-w-sm text-center text-muted-foreground text-sm">
            This agent has no terminal view attached right now.
          </p>
        </div>
      )}

      {presentation !== 'chat' && attachment.state === 'reviving' && (
        <div className="flex h-full w-full flex-col items-center justify-center gap-3 p-6">
          <FlickerSpinner className="size-6 text-foreground" />
          <p className="text-muted-foreground text-center text-sm">{attachment.message}</p>
        </div>
      )}

      {presentation !== 'chat' && attachment.state === 'idle' && (
        <div className="flex h-full w-full flex-col items-center justify-center gap-3 p-6">
          <p className="max-w-sm text-center text-muted-foreground text-sm">
            {offline ? 'Offline — a session can start once the host is back.' : attachment.message}
          </p>
          <Button
            type="button"
            variant="secondary"
            size="sm"
            data-testid="pane-resume"
            disabled={offline}
            onClick={onStartSession}
          >
            Start session
          </Button>
        </div>
      )}

      {/* Not drawn on the chat surface, whose underbar already says this. In the
          pane's header row when it has one, so the terminal keeps the height. */}
      {presentation !== 'chat' &&
        (headerSlot ? (
          createPortal(controls, headerSlot)
        ) : (
          <div
            // The phone keeps the switcher above the grid, clear of the keyboard and key bar.
            className={cn('flex items-center justify-end gap-3 py-2', IS_MOBILE && 'order-first')}
            style={{ paddingRight: gridSlack }}
          >
            {controls}
          </div>
        ))}
    </div>
  )
}
