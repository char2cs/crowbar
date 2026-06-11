import { useEffect } from 'react'
import { splitActiveEditorGroup } from '../utils/pane-command-actions'
import { useWorkspaceStore } from '@/features/workspace/stores/workspace-context'
import { useEffectiveChordMap } from '@/features/keymaps/hooks/use-effective-keymap'
import { eventMatchesChord } from '@/features/keymaps/utils/chord'
import {
  PANE_NAVIGATE_DOWN,
  PANE_NAVIGATE_LEFT,
  PANE_NAVIGATE_RIGHT,
  PANE_NAVIGATE_UP,
  PANE_SPLIT_DOWN,
  PANE_SPLIT_RIGHT,
  TAB_REOPEN_CLOSED,
} from '@/features/keymaps/registry'

/**
 * Pane/tab keyboard shortcuts. All chords are resolved from the keymap registry
 * (the single source of truth surfaced in the Keybindings settings tab), so
 * switching presets or rebinding a command updates these live. Behavior under
 * the Default preset is identical to the previous hardcoded literals.
 */
export function usePaneKeyboard() {
  const workspaceStore = useWorkspaceStore()
  const chordMap = useEffectiveChordMap()

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      const matches = (commandId: string): boolean => {
        const chord = chordMap[commandId]
        return chord ? eventMatchesChord(e, chord) : false
      }

      if (matches(PANE_SPLIT_RIGHT)) {
        e.preventDefault()
        splitActiveEditorGroup('horizontal')
        return
      }

      if (matches(PANE_SPLIT_DOWN)) {
        e.preventDefault()
        splitActiveEditorGroup('vertical')
        return
      }

      if (matches(TAB_REOPEN_CLOSED)) {
        e.preventDefault()
        workspaceStore.getState().bufferActions.reopenLastClosedBuffer()
        return
      }

      const navTargets: Array<[string, 'left' | 'right' | 'up' | 'down']> = [
        [PANE_NAVIGATE_LEFT, 'left'],
        [PANE_NAVIGATE_RIGHT, 'right'],
        [PANE_NAVIGATE_UP, 'up'],
        [PANE_NAVIGATE_DOWN, 'down'],
      ]
      for (const [commandId, direction] of navTargets) {
        if (matches(commandId)) {
          e.preventDefault()
          workspaceStore.getState().paneActions.navigateToPane(direction)
          return
        }
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [workspaceStore, chordMap])
}
