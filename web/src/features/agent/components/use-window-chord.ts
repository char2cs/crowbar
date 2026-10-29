import { useEffect, useEffectEvent } from 'react'
import { eventMatchesChord } from '@/features/keymaps/utils/chord'
import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'

/**
 * A chord this pane answers while the user is looking at it. Capture phase,
 * because a focused xterm stops the keys it handles. A retained workspace stays
 * mounted hidden, so the active workspace is asked inside the handler.
 */
export function useWindowChord(
  chord: string | undefined,
  enabled: boolean,
  wsId: string,
  onChord: () => void,
) {
  const fire = useEffectEvent(onChord)
  useEffect(() => {
    if (!enabled || !chord) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (!eventMatchesChord(event, chord)) return
      // A hidden workspace's pane must neither act nor swallow the key.
      if (getActiveWorkspaceId() !== wsId) return
      event.preventDefault()
      event.stopPropagation()
      fire()
    }
    window.addEventListener('keydown', onKeyDown, true)
    return () => window.removeEventListener('keydown', onKeyDown, true)
  }, [enabled, chord, wsId])
}
