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
  TAB_CLOSE,
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

      if (matches(TAB_CLOSE)) {
        // Always preventDefault so the chord never reaches the OS window-close.
        // Mirror the tab × button (handleTabClose): remove the buffer from its
        // pane FIRST so an adjacent tab activates (raw closeBuffer alone leaves
        // a dangling activeBufferId → empty state), and prompt before discarding
        // a dirty editor buffer. No active buffer → no-op (never quits the app).
        e.preventDefault()
        const state = workspaceStore.getState()
        const paneId = state.activePaneId
        const bufferId = state.panes[paneId]?.activeBufferId
        if (!bufferId) return
        const buf = state.buffers.find((b) => b.id === bufferId)
        if (buf && buf.type === 'editor' && buf.isDirty) {
          state.bufferActions.setPendingClose({ type: 'single', bufferId })
          return
        }
        state.paneActions.removeBufferFromPane(paneId, bufferId)
        state.bufferActions.closeBuffer(bufferId)
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
