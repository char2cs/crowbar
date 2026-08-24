import { ArrowUpIcon, SquareIcon } from 'lucide-react'
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
  onSend: () => void
  onStop: () => void
}

/**
 * The circle at the end of the last line.
 *
 * It is a SEND button whenever there is something to send. With an empty box and
 * a turn in flight it becomes the stop control, because that is the only thing
 * left to do to a running turn — and because a permanently disabled send circle
 * wastes the one affordance a person's hand is already on.
 */
export function ComposerHandle({
  fieldHeight,
  hasText,
  working,
  canStop,
  onSend,
  onStop,
}: ComposerHandleProps) {
  const stopping = !hasText && working && canStop
  const idle = !hasText && !stopping

  return (
    <div className="handle" style={{ transform: `translateY(${handleOffset(fieldHeight)}px)` }}>
      <button
        type="button"
        className={cn('send', stopping && 'halt', idle && 'off')}
        disabled={idle}
        aria-label={stopping ? 'Stop this turn' : 'Send prompt'}
        title={stopping ? 'Stop this turn — Esc' : 'Send — Enter'}
        onClick={stopping ? onStop : onSend}
      >
        {stopping ? <SquareIcon fill="currentColor" /> : <ArrowUpIcon />}
      </button>
    </div>
  )
}
