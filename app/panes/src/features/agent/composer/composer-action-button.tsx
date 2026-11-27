import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { QueueIcon, StopIcon, UpIcon } from '@/features/agent/shared/agent-icons'
import { cn } from '@/lib/utils'

interface ComposerActionButtonProps {
  hasText: boolean
  working: boolean
  canStop: boolean
  sending: boolean
  onSend: () => void
  onStop?: () => void
}

export function ComposerActionButton({
  hasText,
  working,
  canStop,
  sending,
  onSend,
  onStop,
}: ComposerActionButtonProps) {
  const stopping = (working || sending) && canStop && !hasText
  const queueing = hasText && (working || (sending && canStop))
  const sendingVisual = !stopping && !hasText && sending
  const idle = !stopping && !sendingVisual && !hasText
  const inert = idle || sendingVisual

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
      className={cn('send', stopping && 'halt', queueing && 'queue', inert && 'off')}
      disabled={inert}
      aria-label={label}
      title={title}
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
