import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { QueueIcon, StopIcon, UpIcon } from '@/features/agent/shared/agent-icons'
import { cn } from '@/lib/utils'

interface ComposerActionButtonProps {
  hasText: boolean
  working: boolean
  /** The running turn can be interrupted, or the prompt being taken up taken back. */
  canStop: boolean
  /** A prompt has been dispatched but the ledger has not yet proven it delivered. */
  sending: boolean
  onSend: () => void
  /** Why a send goes nowhere yet. The button looks blocked but still takes the click, so the
   *  caller can show what is missing. */
  blockedReason?: string
  onStop?: () => void
}

/** The one circle at the right end of the box: Stop while a turn can be interrupted and the box
 *  is empty, Queue once text is typed into a running turn, otherwise Send. Sending gets its own
 *  look so the gap before delivery doesn't read as a dead click. */
export function ComposerActionButton({
  hasText,
  working,
  canStop,
  sending,
  onSend,
  blockedReason,
  onStop,
}: ComposerActionButtonProps) {
  const stopping = (working || sending) && canStop && !hasText
  const queueing = hasText && (working || (sending && canStop))
  const sendingVisual = !stopping && !hasText && sending
  const idle = !stopping && !sendingVisual && !hasText
  const inert = idle || sendingVisual
  const blocked = blockedReason !== undefined && !inert && !stopping

  const label = stopping
    ? 'Stop this turn'
    : queueing
      ? 'Queue this message'
      : sendingVisual
        ? 'Sending'
        : 'Send prompt'
  const title = stopping
    ? 'Stop this turn — Esc'
    : queueing
      ? 'Queue — sends when this turn ends'
      : sendingVisual
        ? 'Sending…'
        : 'Send — Enter'

  return (
    <button
      type="button"
      className={cn('send', stopping && 'halt', queueing && 'queue', (inert || blocked) && 'off')}
      disabled={inert}
      aria-disabled={blocked || undefined}
      aria-description={blocked ? blockedReason : undefined}
      aria-label={label}
      title={blocked ? blockedReason : title}
      // Keeps the editor focused, so tapping send does not drop the keyboard.
      onMouseDown={(e) => e.preventDefault()}
      onClick={stopping ? onStop : onSend}
    >
      {stopping ? (
        <StopIcon size={16} />
      ) : queueing ? (
        <QueueIcon size={16} />
      ) : sendingVisual ? (
        <FlickerSpinner className="size-4" />
      ) : (
        <UpIcon size={16} />
      )}
    </button>
  )
}
