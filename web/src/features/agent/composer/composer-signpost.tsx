import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { TerminalIcon } from '@/features/agent/shared/agent-icons'
import { Button } from '@/components/ui/button'
import type { AgentTerminalAnswer } from '@/features/agent/api/agent-chat-reads'
import type { SignpostReason } from '@/features/agent/composer/lib/composer-state'

interface ComposerSignpostProps {
  reason: SignpostReason
  message: string
  onOpenTerminal: () => void
  /** The prompt's declared one-tap replies, when it has any. */
  answers?: AgentTerminalAnswer[]
  onAnswer?: (answerId: string) => void
}

/** Takes the composer's slot rather than sitting above it: what it reports is
 *  that there is nothing to type into. */
export function ComposerSignpost({
  reason,
  message,
  onOpenTerminal,
  answers,
  onAnswer,
}: ComposerSignpostProps) {
  return (
    <div className="pill halted" role="status">
      {reason === 'reviving' && <FlickerSpinner className="size-4 text-foreground" />}
      <span className="msg">{message}</span>
      {reason === 'terminal_wait' &&
        answers?.map((answer) => (
          <Button key={answer.id} size="xs" onClick={() => onAnswer?.(answer.id)}>
            {answer.label}
          </Button>
        ))}
      {(reason === 'unsupported' || reason === 'terminal_wait') && (
        <Button size="xs" variant="secondary" onClick={onOpenTerminal}>
          <TerminalIcon /> Terminal
        </Button>
      )}
    </div>
  )
}
