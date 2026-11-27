import { createPortal } from 'react-dom'
import type { CSSProperties, MouseEvent, Ref, RefObject } from 'react'
import { Button } from '@/components/ui/button'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { ProviderSwitchDropdown } from '@/features/agent/components/provider-switch-dropdown'
import { useChatHeaderSlotElement } from '@/features/agent/hooks/chat-header-slot'
import { ViewSwitch } from '@/features/agent/controls/view-switch'
import type { ChatPresentation } from '@/features/settings/lib/chat-presentation'
import { LazyXtermTerminal } from '@/features/terminal/components/lazy-terminal'
import type { TerminalFocusHandle } from '@/features/terminal/components/terminal'
import { TerminalKeyBar } from '@/features/terminal/components/terminal-key-bar'
import { cn } from '@/lib/utils'
import { IS_MOBILE } from '@/utils/platform'

export type TerminalAttachment =
  | { state: 'pending' }
  | { state: 'attached'; sessionId: string | null }
  | { state: 'reviving'; message: string }
  | { state: 'idle' }

export interface AgentTerminalSurfaceProps {
  wsId: string
  chatId: string
  paneId: string
  attachment: TerminalAttachment
  presentation: ChatPresentation
  splitting: boolean
  focused: boolean
  basis: number
  isActivePane: boolean
  isVisible: boolean
  gridSlack: number
  providers: AgentProvider[]
  activeProviderId: string
  switchDisabled: boolean
  splitEnabled: boolean
  working?: boolean
  onSwitchProvider: (providerId: string) => void
  onSelectPresentation: (next: ChatPresentation) => void
  onTakeFocus: () => void
  onDeadSpaceMouseDown: (event: MouseEvent<HTMLDivElement>) => void
  terminalRef: RefObject<TerminalFocusHandle | null>
  onStartSession: () => void
  offline: boolean
  headerClearancePx: number
  ref?: Ref<HTMLDivElement>
}

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
  headerClearancePx,
  ref,
}: AgentTerminalSurfaceProps) {
  const provider = providers.find((candidate) => candidate.id === activeProviderId)
  const headerSlot = useChatHeaderSlotElement()
  const controls = (
    <>
      {presentation === 'terminal' && provider?.hasTerminal !== false && (
        <ViewSwitch
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
      role="presentation"
      data-testid="agent-terminal-surface"
      data-surface-focused={splitting ? String(focused) : undefined}
      onFocusCapture={splitting ? onTakeFocus : undefined}
      onMouseDown={splitting ? onDeadSpaceMouseDown : undefined}
      className={cn(
        'flex flex-col',
        splitting
          ? 'relative min-h-0 min-w-0 shrink grow-0'
          : cn('h-full', presentation === 'terminal' ? '' : 'hidden'),
      )}
      style={
        {
          paddingTop: `${headerClearancePx}px`,
          ...(splitting ? { flexBasis: `${basis}%` } : undefined),
        } as CSSProperties
      }
    >
      {attachment.state === 'attached' && attachment.sessionId && (
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

      {IS_MOBILE &&
        presentation === 'terminal' &&
        attachment.state === 'attached' &&
        attachment.sessionId && (
          <TerminalKeyBar onSend={(bytes) => terminalRef?.current?.send(bytes)} />
        )}

      {presentation !== 'chat' && attachment.state === 'attached' && !attachment.sessionId && (
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
          {offline && (
            <p className="max-w-sm text-center text-muted-foreground text-sm">
              Offline — a session can start once the host is back.
            </p>
          )}
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

      {presentation !== 'chat' &&
        (headerSlot ? (
          createPortal(controls, headerSlot)
        ) : (
          <div
            className={cn('flex items-center justify-end gap-3 py-2', IS_MOBILE && 'order-first')}
            style={{ paddingRight: gridSlack }}
          >
            {controls}
          </div>
        ))}
    </div>
  )
}
