import { useEffect } from 'react'
import { useEffectiveChordMap } from '@/features/keymaps/hooks/use-effective-keymap'
import { eventMatchesChord } from '@/features/keymaps/utils/chord'
import { OPEN_WORKSPACE_SWITCHER } from '@/features/keymaps/registry'

export function useWorkspaceSwitcherKeyboard(onOpen: () => void): void {
  const chordMap = useEffectiveChordMap()

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      const chord = chordMap[OPEN_WORKSPACE_SWITCHER]
      if (!chord || !eventMatchesChord(e, chord)) return
      e.preventDefault()
      onOpen()
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [chordMap, onOpen])
}
