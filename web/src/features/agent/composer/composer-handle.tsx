import { ComposerActionButton } from '@/features/agent/composer/composer-action-button'
import { ComposerPlusButton } from '@/features/agent/composer/composer-plus-button'
import { handleOffset } from '@/features/agent/composer/lib/handle-geometry'

interface ComposerHandleProps {
  fieldHeight: number
  hasText: boolean
  working: boolean
  /** The running turn can be interrupted, or the prompt being taken up taken back. */
  canStop: boolean
  /** A prompt has been dispatched but the ledger has not yet proven it delivered. */
  sending: boolean
  onSend: () => void
  onStop: () => void
  onOpenExcalidraw: () => void
  onOpenAttachFile: () => void
}

/** Plus and the action circle, side by side at the right end of the pill, riding the last line. */
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
  return (
    <div className="handle" style={{ transform: `translateY(${handleOffset(fieldHeight)}px)` }}>
      <ComposerPlusButton onOpenExcalidraw={onOpenExcalidraw} onOpenAttachFile={onOpenAttachFile} />
      <ComposerActionButton
        hasText={hasText}
        working={working}
        canStop={canStop}
        sending={sending}
        onSend={onSend}
        onStop={onStop}
      />
    </div>
  )
}
