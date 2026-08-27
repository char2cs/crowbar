import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { TerminalIcon } from '@/features/agent/shared/agent-icons'
import { Button } from '@/components/ui/button'
import type { SignpostReason } from '@/features/agent/composer/lib/composer-state'

interface ComposerSignpostProps {
  reason: SignpostReason
  message: string
  onOpenTerminal: () => void
  /** The manual retry for a revive that already gave up. Only read for `idle`. */
  onRevive?: () => void
}

/**
 * The bar as a REASON YOU CANNOT TYPE.
 *
 * Every one of these is a dead end for the chat surface, and most have a live
 * way out somewhere else, so each carries it rather than only the diagnosis. It
 * occupies the composer's slot instead of sitting above it: the fact it reports
 * IS that there is nothing to type into.
 *
 * `reviving` and `idle` are not redirects to the terminal — Crowbar's own revive
 * is either already running (a spinner, nothing to click) or has already been
 * tried and given up (a manual retry, not a place to go instead).
 */
export function ComposerSignpost({
  reason,
  message,
  onOpenTerminal,
  onRevive,
}: ComposerSignpostProps) {
  return (
    <div className="pill halted" role="status">
      {reason === 'reviving' && <FlickerSpinner className="size-4 text-foreground" />}
      <span className="msg">{message}</span>
      {reason === 'idle' && onRevive && (
        <Button size="xs" variant="secondary" data-testid="pane-resume" onClick={onRevive}>
          Resume
        </Button>
      )}
      {(reason === 'unsupported' || reason === 'terminal_wait') && (
        <Button size="xs" variant="secondary" onClick={onOpenTerminal}>
          <TerminalIcon /> Terminal
        </Button>
      )}
    </div>
  )
}
