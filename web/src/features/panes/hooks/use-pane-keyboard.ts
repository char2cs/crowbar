import { useEffect } from 'react'
import { splitActiveEditorGroup } from '../utils/pane-command-actions'
import { getPaneScopeForPaneId } from '../utils/pane-routing'
import { useWorkspaceStore } from '@/features/workspace/stores/workspace-context'
import { useEffectiveChordMap } from '@/features/keymaps/hooks/use-effective-keymap'
import { eventMatchesChord } from '@/features/keymaps/utils/chord'
import { createChat } from '@/features/agent/api/agent-api'
import { selectEnabledProviders } from '@/features/workspace/stores/slices/agent-chats-slice'
import { toastSpawnFailure } from '@/features/agent/lib/spawn-error'
import {
  AGENT_NEW_CHAT,
  PANE_NAVIGATE_DOWN,
  PANE_NAVIGATE_LEFT,
  PANE_NAVIGATE_RIGHT,
  PANE_NAVIGATE_UP,
  PANE_SPLIT_DOWN,
  PANE_SPLIT_RIGHT,
  TAB_CLOSE,
  TAB_NEW,
  TAB_NEW_FILE,
  TAB_NEW_TERMINAL,
  TAB_REOPEN_CLOSED,
} from '@/features/keymaps/registry'

/**
 * Pane/tab keyboard shortcuts. All chords are resolved from the keymap registry
 * (the single source of truth surfaced in the Keybindings settings tab), so
 * switching presets or rebinding a command updates these live. `mod+t` opens a
 * New Tab (not a terminal, and not the previous hardcoded literal it replaced).
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

      if (matches(TAB_NEW)) {
        e.preventDefault()
        workspaceStore.getState().bufferActions.openNewTab()
        return
      }

      if (matches(TAB_NEW_TERMINAL)) {
        e.preventDefault()
        workspaceStore.getState().bufferActions.openContent({ type: 'terminal' })
        return
      }

      if (matches(TAB_NEW_FILE)) {
        e.preventDefault()
        workspaceStore.getState().bufferActions.openContent({
          type: 'editor',
          path: 'untitled:Untitled',
          name: 'Untitled',
          content: '',
          isVirtual: true,
        })
        return
      }

      if (matches(AGENT_NEW_CHAT)) {
        // ⌘N is registered in the keymap and rendered as a badge on the New
        // Tab surface's "New Chat" action, but nothing dispatched it (I4) — a
        // rebindable command whose chord did nothing. Mirrors NewTabView's own
        // createNewChat: pick the first ENABLED provider (selectEnabledProviders —
        // the same rule every New-chat surface uses; a disabled provider is never
        // offered), create the chat, then open it on the currently active pane.
        e.preventDefault()
        const state = workspaceStore.getState()
        const provider = selectEnabledProviders(state)[0]
        if (!provider) return
        const targetPaneId = state.activePaneId
        createChat(state.workspaceId, provider.id)
          .then((chatId) => {
            const st = workspaceStore.getState()
            const title = st.agentChats.chats.find((c) => c.id === chatId)?.title
            st.setActiveAgentChatId(chatId)
            st.paneActions.setActivePane(targetPaneId)
            st.bufferActions.openContent({
              type: 'agentChat',
              chatId,
              wsId: st.workspaceId,
              name: title || `${provider.displayName} chat`,
            })
          })
          .catch((err: unknown) => toastSpawnFailure(err, provider.displayName, 'start'))
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
        // A sole New Tab has no close: closing it would spawn another (see
        // pane-slice's removeBufferFromPane), so ⌘W would visibly do nothing.
        // In a SPLIT that keystroke means "dismiss this split" — which is the
        // only reading that leaves the user anywhere new. In the last remaining
        // pane there is nowhere to go, so it is a genuine no-op.
        //
        // "Last remaining pane" must be scoped to paneId's OWN layout tree
        // (root editor area vs. the bottom panel) via getPaneScopeForPaneId —
        // `state.panes` always additionally holds the OTHER tree's root
        // (ROOT_PANE_ID + BOTTOM_PANE_ID both exist even in a single-pane
        // workspace), so a raw `Object.keys(state.panes).length` is never 1 and
        // this branch would always read as "there's a split", closing the sole
        // remaining pane in either tree and bricking it (see pane-slice's
        // closePane — deleting the sole root leaf goes through the reseed
        // branch and immediately deletes the fallback leaf right after).
        const pane = state.panes[paneId]
        if (buf?.type === 'newTab' && pane && pane.bufferIds.length === 1) {
          const scope = getPaneScopeForPaneId(
            state.rootLayout,
            state.bottomLayout,
            state.panes,
            paneId,
          )
          if (scope.length > 1) {
            state.paneActions.closePane(paneId)
          }
          return
        }
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
