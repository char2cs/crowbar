import type { ReactNode } from 'react'
import { ComposerActionButton } from '@/features/agent/composer/composer-action-button'
import { handleOffset } from '@/features/agent/composer/lib/handle-geometry'

interface ComposerHandleProps {
  fieldHeight: number
  hasText: boolean
  working: boolean
  canStop: boolean
  sending: boolean
  onSend: () => void
  onStop: () => void
  attach: ReactNode
}

export function ComposerHandle({
  fieldHeight,
  hasText,
  working,
  canStop,
  sending,
  onSend,
  onStop,
  attach,
}: ComposerHandleProps) {
  return (
    <div className="handle" style={{ transform: `translateY(${handleOffset(fieldHeight)}px)` }}>
      {attach}
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
