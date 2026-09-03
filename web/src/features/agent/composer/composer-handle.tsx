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
 * STOP WINS whenever a turn is actually stoppable, text in the box or not.
 * Typing a follow-up while a turn runs is exactly the case "Queue a message…"
 * invites — and a box that hides the only way to interrupt the turn the moment
 * there's a character in it leaves nothing to click: Enter still queues that
 * text regardless of what this button shows, so nothing typed is lost by
 * letting the button read as Stop instead. Between idle and stopping it can
 * also be SENDING: the draft clears the instant a prompt queues, well before
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
  const stopping = working && canStop
  const sendingVisual = !stopping && !hasText && sending
  const idle = !stopping && !sendingVisual && !hasText

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
