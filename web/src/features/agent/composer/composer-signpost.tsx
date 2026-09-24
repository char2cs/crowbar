import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { TerminalIcon } from '@/features/agent/shared/agent-icons'
import { Button } from '@/components/ui/button'
import type { SignpostReason } from '@/features/agent/composer/lib/composer-state'

interface ComposerSignpostProps {
  reason: SignpostReason
  message: string
  onOpenTerminal: () => void
}

/**
 * The bar as a REASON YOU CANNOT TYPE.
 *
 * It occupies the composer's slot instead of sitting above it: the fact it
 * reports IS that there is nothing to type into. Where there is a way out
 * elsewhere (the terminal), it carries it. `reviving` is the daemon placing a
 * CLI — a spinner, nothing to click.
 */
export function ComposerSignpost({ reason, message, onOpenTerminal }: ComposerSignpostProps) {
  return (
    <div className="pill halted" role="status">
      {reason === 'reviving' && <FlickerSpinner className="size-4 text-foreground" />}
      <span className="msg">{message}</span>
      {(reason === 'unsupported' || reason === 'terminal_wait') && (
        <Button size="xs" variant="secondary" onClick={onOpenTerminal}>
          <TerminalIcon /> Terminal
        </Button>
      )}
    </div>
  )
}
