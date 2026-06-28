import { useEffect } from 'react'
import { useEffectiveChordMap } from '@/features/keymaps/hooks/use-effective-keymap'
import { eventMatchesChord } from '@/features/keymaps/utils/chord'
import { OPEN_WORKSPACE_SWITCHER } from '@/features/keymaps/registry'

export function useWorkspaceSwitcherKeyboard(onOpen: () => void): void {
  const chordMap = useEffectiveChordMap()

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.repeat) return
      const chord = chordMap[OPEN_WORKSPACE_SWITCHER]
      if (!chord || !eventMatchesChord(e, chord)) return
      // stopPropagation prevents Monaco from entering chord-pending state after
      // we handle the shortcut (Monaco calls stopPropagation on Cmd+K as a chord
      // prefix, so capture phase is required to win over Monaco's bubble handler).
      e.preventDefault()
      e.stopPropagation()
      onOpen()
    }

    window.addEventListener('keydown', handleKeyDown, true)
    return () => window.removeEventListener('keydown', handleKeyDown, true)
  }, [chordMap, onOpen])
}
