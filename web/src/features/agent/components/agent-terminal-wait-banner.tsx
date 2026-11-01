import { TerminalIcon } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { TERMINAL_WAIT_TRUST } from '@/features/agent/api/agent-chat-reads'

interface AgentTerminalWaitBannerProps {
  /** Crowbar's own name for the prompt, or '' when it knows only that one is up. */
  kind: string
  /** The provider's display name, or '' when it is not known yet. */
  providerLabel: string
  onOpenTerminal: () => void
}

/** The CLI is blocked on a prompt with no hook channel (trust, login), so this
 *  points at the terminal instead of offering to answer. Pane-level because the
 *  composer is unreachable before the first turn; one line so its height is fixed. */
export function AgentTerminalWaitBanner({
  kind,
  providerLabel,
  onOpenTerminal,
}: AgentTerminalWaitBannerProps) {
  const who = providerLabel || 'The agent'
  const what =
    kind === TERMINAL_WAIT_TRUST
      ? `${who} is asking whether you trust this folder.`
      : `${who} is waiting for input in its terminal.`

  return (
    <div
      className="flex h-[38px] items-center gap-3 rounded-full border border-warning/40 bg-warning/10 px-3.5 text-sm"
      role="alert"
      data-testid="agent-terminal-wait"
      data-wait-kind={kind}
    >
      <p className="min-w-0 truncate text-warning-foreground">
        {what} <span className="text-muted-foreground">Crowbar can’t answer this one.</span>
      </p>
      <Button className="ml-auto shrink-0" size="xs" variant="secondary" onClick={onOpenTerminal}>
        <TerminalIcon /> Open Terminal
      </Button>
    </div>
  )
}

interface AgentReturnToChatNoticeProps {
  onReturn: () => void
  onDismiss: () => void
}

/** Offered instead of an automatic return once the user has navigated or picked a
 *  surface themselves: yanking them out of a terminal they are using is worse. */
export function AgentReturnToChatNotice({ onReturn, onDismiss }: AgentReturnToChatNoticeProps) {
  return (
    <div
      className="flex items-center justify-between gap-3 border-t py-2 text-muted-foreground text-xs"
      role="status"
      data-testid="agent-return-to-chat"
    >
      <span>The agent is no longer waiting on you here.</span>
      <div className="flex items-center gap-1">
        <Button size="xs" variant="ghost" onClick={onReturn}>
          Return to Chat
        </Button>
        <Button size="xs" variant="ghost" onClick={onDismiss}>
          Dismiss
        </Button>
      </div>
    </div>
  )
}
