import { TerminalIcon } from 'lucide-react'
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
 * Every one of these is a dead end for the chat surface and a live one somewhere
 * else, so each carries the way out rather than only the diagnosis. It occupies
 * the composer's slot instead of sitting above it: the fact it reports IS that
 * there is nothing to type into.
 */
export function ComposerSignpost({ reason, message, onOpenTerminal }: ComposerSignpostProps) {
  return (
    <div className="pill halted" role="status">
      <span className="msg">{message}</span>
      {/* A dormant chat already has the pane's own banner above the transcript,
          carrying the reason AND the way back. Repeating the button here would be
          two of the same action on one surface. */}
      {reason !== 'dormant' && (
        <Button size="xs" variant="secondary" onClick={onOpenTerminal}>
          <TerminalIcon /> Terminal
        </Button>
      )}
    </div>
  )
}
