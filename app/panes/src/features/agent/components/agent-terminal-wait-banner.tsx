import { TerminalIcon } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import {
  TERMINAL_WAIT_TRUST,
  type AgentTerminalAnswer,
} from '@/features/agent/api/agent-chat-reads'

interface AgentTerminalWaitBannerProps {
  /** Crowbar's own name for the prompt, or '' when it knows only that one is up. */
  kind: string
  /** The provider's display name, or '' when it is not known yet. */
  providerLabel: string
  onOpenTerminal: () => void
  /** The prompt's declared one-tap replies, when it has any. */
  answers?: AgentTerminalAnswer[]
  onAnswer?: (answerId: string) => void
}

/** The CLI is blocked on a prompt with no hook channel (trust, login): it offers the
 *  descriptor's declared replies, else points at the terminal. Pane-level because the
 *  composer is unreachable before the first turn; one line except on touch, which wraps. */
export function AgentTerminalWaitBanner({
  kind,
  providerLabel,
  onOpenTerminal,
  answers,
  onAnswer,
}: AgentTerminalWaitBannerProps) {
  const who = providerLabel || 'The agent'
  const what =
    kind === TERMINAL_WAIT_TRUST
      ? `${who} is asking whether you trust this folder.`
      : `${who} is waiting for input in its terminal.`

  return (
    <div
      className="flex h-[38px] items-center gap-3 rounded-full pointer-coarse:h-auto pointer-coarse:min-h-[38px] pointer-coarse:flex-wrap pointer-coarse:justify-end pointer-coarse:gap-y-2 pointer-coarse:rounded-[22px] pointer-coarse:py-2.5 border border-warning/40 bg-warning/10 px-3.5 text-sm"
      role="alert"
      data-testid="agent-terminal-wait"
      data-wait-kind={kind}
    >
      <p className="min-w-0 truncate text-warning-foreground dark:text-warning pointer-coarse:flex-[1_1_14rem] pointer-coarse:whitespace-normal pointer-coarse:overflow-visible">
        {what} <span className="text-muted-foreground">Crowbar can’t answer this one.</span>
      </p>
      <div className="ml-auto flex shrink-0 items-center gap-1.5">
        {answers?.map((answer) => (
          <Button key={answer.id} size="xs" onClick={() => onAnswer?.(answer.id)}>
            {answer.label}
          </Button>
        ))}
        <Button size="xs" variant="secondary" onClick={onOpenTerminal}>
          <TerminalIcon /> Open Terminal
        </Button>
      </div>
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
