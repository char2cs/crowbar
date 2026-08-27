import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { StopIcon, UpIcon } from '@/features/agent/shared/agent-icons'
import { cn } from '@/lib/utils'
import { handleOffset } from '@/features/agent/composer/lib/handle-geometry'

interface ComposerHandleProps {
  /** The field's measured height, in px. */
  fieldHeight: number
  /** There is text to send. */
  hasText: boolean
  /** A turn is in flight. */
  working: boolean
  /** The provider declares an interrupt Crowbar can send. */
  canStop: boolean
  /** A prompt has been dispatched but the ledger has not yet proven it delivered. */
  sending: boolean
  onSend: () => void
  onStop: () => void
}

/**
 * The circle at the end of the last line.
 *
 * It is a SEND button whenever there is something to send. With an empty box and
 * a turn in flight it becomes the stop control, because that is the only thing
 * left to do to a running turn — and because a permanently disabled send circle
 * wastes the one affordance a person's hand is already on. Between those two it
 * can also be SENDING: the draft clears the instant a prompt queues, well before
 * the server has proven delivery, and with no distinct look here that gap reads
 * as the click having done nothing.
 */
export function ComposerHandle({
  fieldHeight,
  hasText,
  working,
  canStop,
  sending,
  onSend,
  onStop,
}: ComposerHandleProps) {
  const stopping = !hasText && working && canStop
  // Stopping wins when both apply: a real interrupt is worth more than a spinner
  // that only says something is in flight.
  const sendingVisual = !hasText && !stopping && sending
  const idle = !hasText && !stopping && !sendingVisual

  return (
    <div className="handle" style={{ transform: `translateY(${handleOffset(fieldHeight)}px)` }}>
      <button
        type="button"
        className={cn('send', stopping && 'halt', (idle || sendingVisual) && 'off')}
        disabled={idle || sendingVisual}
        aria-label={stopping ? 'Stop this turn' : sendingVisual ? 'Sending' : 'Send prompt'}
        title={stopping ? 'Stop this turn — Esc' : sendingVisual ? 'Sending…' : 'Send — Enter'}
        onClick={stopping ? onStop : onSend}
      >
        {stopping ? (
          <StopIcon size={16} />
        ) : sendingVisual ? (
          <FlickerSpinner className="size-4" />
        ) : (
          <UpIcon size={16} />
        )}
      </button>
    </div>
  )
}
