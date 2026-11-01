import type { CSSProperties, MouseEvent, Ref } from 'react'
import { Button } from '@/components/ui/button'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { ProviderSwitchDropdown } from '@/features/agent/components/provider-switch-dropdown'
import { ViewSwitcher } from '@/features/agent/controls/view-switcher'
import type { ChatPresentation } from '@/features/settings/lib/chat-presentation'
import {
  LazyXtermTerminal,
  type XtermTerminalProps,
} from '@/features/terminal/components/lazy-terminal'
import { cn } from '@/lib/utils'

/**
 * `pending` is not idle: offering Resume there would spawn a second CLI. A null
 * sessionId is a live runner with no terminal (codex), not dormancy.
 */
export type TerminalAttachment =
  | { state: 'pending' }
  | { state: 'attached'; sessionId: string | null }
  | { state: 'reviving'; message: string }
  | { state: 'idle'; message: string }

export interface AgentTerminalSurfaceProps {
  wsId: string
  /** The CLI session is owned by the chat, not the worktree sibling chats share. */
  chatId: string
  attachment: TerminalAttachment
  presentation: ChatPresentation
  splitting: boolean
  /** This half holds the keyboard. Only meaningful while splitting. */
  focused: boolean
  /** Percentage of the split this half occupies. */
  basis: number
  isActivePane: boolean
  isVisible: boolean
  title: string
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
  terminalRef: XtermTerminalProps['ref']
  /** Bring the chat's provider back without sending anything. */
  onStartSession: () => void
  ref?: Ref<HTMLDivElement>
}

/** A sibling of the chat surface, not a variant: they share only the surface
 *  switcher, the one fact that is the same on both. */
export function AgentTerminalSurface({
  wsId,
  chatId,
  attachment,
  presentation,
  splitting,
  focused,
  basis,
  isActivePane,
  isVisible,
  title,
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
  ref,
}: AgentTerminalSurfaceProps) {
  const provider = providers.find((candidate) => candidate.id === activeProviderId)
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
          isActive={
            isActivePane && isVisible && (presentation === 'terminal' || (splitting && focused))
          }
          isVisible={isVisible && presentation !== 'chat'}
          attachOnly
          flush
          ref={terminalRef}
        />
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
          <p className="max-w-sm text-center text-muted-foreground text-sm">{attachment.message}</p>
          <Button
            type="button"
            variant="secondary"
            size="sm"
            data-testid="pane-resume"
            onClick={onStartSession}
          >
            Start session
          </Button>
        </div>
      )}

      {/* Not drawn on the chat surface, whose underbar already says this. `min-w-0`
          lets a long title truncate instead of shoving the switcher off the grid. */}
      {presentation !== 'chat' && (
        <div
          className="flex items-center justify-between gap-3 py-2"
          style={{ paddingRight: gridSlack }}
        >
          <span className="min-w-0 truncate text-muted-foreground text-sm">{title}</span>
          {/* One per pane: in split the chat's provider bar carries it. */}
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
          />
        </div>
      )}
    </div>
  )
}
