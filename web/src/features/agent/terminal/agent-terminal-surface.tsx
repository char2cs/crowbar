import { createPortal } from 'react-dom'
import type { CSSProperties, MouseEvent, Ref } from 'react'
import { Button } from '@/components/ui/button'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import type { AgentProvider } from '@/features/agent/api/agent-api'
import { ProviderSwitchDropdown } from '@/features/agent/components/provider-switch-dropdown'
import { useChatHeaderSlotElement } from '@/features/agent/hooks/chat-header-slot'
import { ViewSwitcher } from '@/features/agent/controls/view-switcher'
import type { ChatPresentation } from '@/features/settings/lib/chat-presentation'
import {
  LazyXtermTerminal,
  type XtermTerminalProps,
} from '@/features/terminal/components/lazy-terminal'
import { cn } from '@/lib/utils'

/**
 * What the pane knows about the runner this surface is looking at.
 *
 * `pending` is "not answered yet", which is NOT the same as idle: a chat whose
 * liveness has not come back must not be offered a Resume button, or the pane
 * spawns a second CLI onto a chat that already has one.
 *
 * `attached`'s sessionId is null for a runner that IS live but has no terminal
 * to show right now — a non-hotswap api-transport provider (codex) between
 * switches, never attached. That is not dormancy: the chat itself is fine, and
 * offering Resume here would restart an agent that was never down.
 */
export type TerminalAttachment =
  | { state: 'pending' }
  | { state: 'attached'; sessionId: string | null }
  | { state: 'reviving'; message: string }
  | { state: 'idle'; message: string }

export interface AgentTerminalSurfaceProps {
  wsId: string
  /**
   * The chat this PTY belongs to. Terminal routes are chat-scoped, and an
   * agent chat's vendor-CLI session is owned by the chat itself — not by the
   * worktree it runs in, which sibling chats share.
   */
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
  /** A turn is in flight — gates handover back to chat the same way the chat
   *  surface's own switcher gates handover TO the terminal. */
  working?: boolean
  onSwitchProvider: (providerId: string) => void
  onSelectPresentation: (next: ChatPresentation) => void
  onTakeFocus: () => void
  /** A click on this half's dead space, pointed back at the grid. */
  onDeadSpaceMouseDown: (event: MouseEvent<HTMLDivElement>) => void
  /** The terminal's focus handle, for moving the keyboard onto the grid. */
  terminalRef: XtermTerminalProps['ref']
  /** Bring the chat's provider back without sending anything. */
  onStartSession: () => void
  ref?: Ref<HTMLDivElement>
}

/**
 * THE PROVIDER'S OWN VIEW.
 *
 * The CLI's real TUI, plus the one strip of chrome that belongs to it: what this
 * conversation is called, which face of the provider you are looking at, and
 * which agent is running it.
 *
 * A SIBLING of the chat surface, not a variant of it. The two answer the same
 * question in genuinely different ways — one reconstructs the conversation from
 * hooks, the other shows the process — and they share exactly one component,
 * the surface switcher, because that is the only thing that is the same fact on
 * both. Everything else here is the terminal's and nothing else's.
 */
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
      // Layout chrome: the mousedown handler is whitelisted to clicks that land
      // on THIS div and nowhere else — the strip of the half the character grid
      // does not reach — so it is dead space being pointed back at the terminal,
      // not a control. role="presentation" says so; the terminal keeps its own.
      role="presentation"
      data-testid="agent-terminal-surface"
      data-surface-focused={splitting ? String(focused) : undefined}
      onFocusCapture={splitting ? onTakeFocus : undefined}
      onMouseDown={splitting ? onDeadSpaceMouseDown : undefined}
      className={cn(
        // A COLUMN: the grid takes the slack and the status strip keeps its
        // height underneath, which is what stops the strip from being laid out
        // over the terminal it describes.
        'flex flex-col',
        splitting
          ? 'relative min-h-0 min-w-0 shrink grow-0'
          : cn('h-full', presentation === 'terminal' ? '' : 'hidden'),
      )}
      style={splitting ? ({ flexBasis: `${basis}%` } as CSSProperties) : undefined}
    >
      {attachment.state === 'attached' && attachment.sessionId && (
        // NO key={sessionId}. Runner replacement swaps the PTY imperatively
        // instead of rebuilding xterm; runner movement keeps the same PTY.
        //
        // isActive IS FOCUS, not liveness — it is what makes xterm grab the
        // caret back, with retries. In split it is therefore gated on this half
        // actually holding the keyboard, or a user typing into the composer
        // would lose the rest of their sentence to the TUI. isVisible is the
        // liveness half, and in split it is simply true: both surfaces really
        // are on screen. Both still hang off the pane's own axes, so a split in
        // a hidden tab stays as dormant as one in terminal mode does.
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

      {presentation !== 'chat' && attachment.state === 'attached' && !attachment.sessionId && (
        // A live, non-hotswap api-transport runner (codex) with nothing attached —
        // correct and common, not a failure. No Resume button: there is nothing to
        // restart.
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

      {/* Which face of the provider you are looking at, and which agent runs it.
          Drawn in the pane's header row when it has one, so the terminal keeps
          the height; a pane with no header row keeps them on a strip here.

          NOT DRAWN ON THE CHAT SURFACE. Chat states what it is running as in its
          own underbar, on the composer's measure; repeating it under a surface
          that already says all of it is two answers to one question. */}
      {presentation !== 'chat' &&
        (headerSlot ? (
          createPortal(controls, headerSlot)
        ) : (
          <div
            className="flex items-center justify-end gap-3 py-2"
            style={{ paddingRight: gridSlack }}
          >
            {controls}
          </div>
        ))}
    </div>
  )
}
