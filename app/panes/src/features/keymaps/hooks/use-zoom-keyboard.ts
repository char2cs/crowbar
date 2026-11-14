// Chat zoom chords (mod+= / mod+- / mod+0 by default) resolve from the keymap registry, so a
// preset switch or rebind takes effect live.
import { useEffect } from 'react'
import { useEffectiveChordMap } from '@/features/keymaps/hooks/use-effective-keymap'
import { eventMatchesChord } from '@/features/keymaps/utils/chord'
import { AGENT_ZOOM_IN, AGENT_ZOOM_OUT, AGENT_ZOOM_RESET } from '@/features/keymaps/registry'
import { useZoomStore } from '@/features/window/stores/zoom-store'

export function useZoomKeyboard(): void {
  const chordMap = useEffectiveChordMap()

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // One zoom step per press, not per OS key-repeat tick.
      if (e.repeat) return
      const matches = (commandId: string): boolean => {
        const chord = chordMap[commandId]
        return chord ? eventMatchesChord(e, chord) : false
      }

      if (matches(AGENT_ZOOM_IN)) {
        e.preventDefault()
        useZoomStore.getState().actions.zoomIn()
        return
      }

      if (matches(AGENT_ZOOM_OUT)) {
        e.preventDefault()
        useZoomStore.getState().actions.zoomOut()
        return
      }

      if (matches(AGENT_ZOOM_RESET)) {
        e.preventDefault()
        useZoomStore.getState().actions.resetZoom()
        return
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [chordMap])
}
