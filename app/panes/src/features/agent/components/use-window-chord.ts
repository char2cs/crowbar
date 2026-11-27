import { useEffect, useEffectEvent } from 'react'
import { eventMatchesChord } from '@/features/keymaps/utils/chord'
import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'

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
      if (getActiveWorkspaceId() !== wsId) return
      event.preventDefault()
      event.stopPropagation()
      fire()
    }
    window.addEventListener('keydown', onKeyDown, true)
    return () => window.removeEventListener('keydown', onKeyDown, true)
  }, [enabled, chord, wsId])
}
