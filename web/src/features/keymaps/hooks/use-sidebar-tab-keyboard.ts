import { useEffect } from 'react'
import { useEffectiveChordMap } from '@/features/keymaps/hooks/use-effective-keymap'
import { eventMatchesChord } from '@/features/keymaps/utils/chord'
import {
  SIDEBAR_TAB_WORKSPACES,
  SIDEBAR_TAB_CHATS,
  SIDEBAR_TAB_FILES,
  SIDEBAR_TAB_GIT,
} from '@/features/keymaps/registry'
import { useSidebarStore } from '@/lib/store/sidebar'
import type { SidebarTab } from '@/lib/store/sidebar'

const SIDEBAR_TAB_COMMANDS: Array<[string, SidebarTab]> = [
  [SIDEBAR_TAB_WORKSPACES, 'workspaces'],
  [SIDEBAR_TAB_CHATS, 'chats'],
  [SIDEBAR_TAB_FILES, 'files'],
  [SIDEBAR_TAB_GIT, 'git'],
]

export function useSidebarTabKeyboard(): void {
  const chordMap = useEffectiveChordMap()

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.repeat) return
      for (const [commandId, tab] of SIDEBAR_TAB_COMMANDS) {
        const chord = chordMap[commandId]
        if (chord && eventMatchesChord(e, chord)) {
          e.preventDefault()
          useSidebarStore.getState().setActiveTab(tab)
          return
        }
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [chordMap])
}
