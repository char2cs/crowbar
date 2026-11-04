import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { ComposerPlusButton } from '@/features/agent/composer/composer-plus-button'
import { StopIcon, UpIcon } from '@/features/agent/shared/agent-icons'
import { cn } from '@/lib/utils'
import { handleOffset } from '@/features/agent/composer/lib/handle-geometry'
import { IS_MOBILE } from '@/utils/platform'

interface ComposerHandleProps {
  fieldHeight: number
  hasText: boolean
  working: boolean
  /** The provider declares an interrupt Crowbar can send. */
  canStop: boolean
  /** A prompt has been dispatched but the ledger has not yet proven it delivered. */
  sending: boolean
  onSend: () => void
  onStop: () => void
  onOpenExcalidraw: () => void
  onOpenAttachFile: () => void
}

/**
 * Stop wins over send whenever the turn is stoppable — Enter still queues typed
 * text. Sending gets its own look so the gap before delivery doesn't read as a dead click.
 */
export function ComposerHandle({
  fieldHeight,
  hasText,
  working,
  canStop,
  sending,
  onSend,
  onStop,
  onOpenExcalidraw,
  onOpenAttachFile,
}: ComposerHandleProps) {
  // A phone has no Enter to queue with, so typed text makes the button send.
  const stopping = working && canStop && !(IS_MOBILE && hasText)
  const sendingVisual = !stopping && !hasText && sending
  const idle = !stopping && !sendingVisual && !hasText

  return (
    <div className="handle" style={{ transform: `translateY(${handleOffset(fieldHeight)}px)` }}>
      <ComposerPlusButton onOpenExcalidraw={onOpenExcalidraw} onOpenAttachFile={onOpenAttachFile} />
      <button
        type="button"
        className={cn('send', stopping && 'halt', (idle || sendingVisual) && 'off')}
        disabled={idle || sendingVisual}
        aria-label={stopping ? 'Stop this turn' : sendingVisual ? 'Sending' : 'Send prompt'}
        title={stopping ? 'Stop this turn — Esc' : sendingVisual ? 'Sending…' : 'Send — Enter'}
        // Keeps the editor focused, so tapping send does not drop the keyboard.
        onMouseDown={(e) => e.preventDefault()}
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
