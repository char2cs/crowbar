import { useEffect } from 'react'
import { ensurePaneChatThenOpen, openNewTabPage } from '../utils/pane-command-actions'
import { getPaneScopeForPaneId } from '../utils/pane-routing'
import { useWorkspaceStore } from '@/features/workspace/stores/workspace-context'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { showingLayout } from '@/features/panes/lib/view-state'
import { useEffectiveChordMap } from '@/features/keymaps/hooks/use-effective-keymap'
import { eventMatchesChord } from '@/features/keymaps/utils/chord'
import { createChat } from '@/features/agent/api/agent-chat-creates'
import {
  createSurfaceFor,
  providerCanStartOnTerminal,
} from '@/features/agent/api/agent-providers-api'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import { toastSpawnFailure } from '@/features/agent/lib/spawn-error'
import { presetChatLandingPresentation } from '@/features/agent/hooks/use-chat-presentation'
import {
  openDocumentFromDialog,
  openUntitledDocument,
} from '@/features/editor/documents/open-document'
import { isTauri } from '@/lib/crowbar-bridge'
import {
  AGENT_NEW_CHAT,
  AGENT_NEW_CHAT_TERMINAL,
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
  TAB_OPEN_FILE,
  TAB_REOPEN_CLOSED,
} from '@/features/keymaps/registry'

/** The pane whose editor area (pane-container.tsx's `data-editor-view`) holds focus. */
function focusedEditorPaneId(): string | null {
  const view = document.activeElement?.closest('[data-editor-view]')
  return view?.closest('[data-pane-id]')?.getAttribute('data-pane-id') ?? null
}

// Pane/tab shortcuts. Chords resolve from the keymap registry (what the Keybindings tab shows),
// so presets and rebinds apply live. `mod+t` opens a New Tab page in the active pane.
export function usePaneKeyboard() {
  const workspaceStore = useWorkspaceStore()
  const chordMap = useEffectiveChordMap()

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      const matches = (commandId: string): boolean => {
        const chord = chordMap[commandId]
        return chord ? eventMatchesChord(e, chord) : false
      }

      if (matches(PANE_SPLIT_RIGHT) || matches(PANE_SPLIT_DOWN)) {
        // A pane groups chats, never tabs (spec §7.3), so a tab can't be split into a pane.
        // The command stays registered so its chord is claimed, not passed to the browser/OS.
        e.preventDefault()
        return
      }

      // Over an editor area ⌘N (New Chat's chord) means New Tab there instead.
      const editorPaneId = matches(AGENT_NEW_CHAT) ? focusedEditorPaneId() : null
      if (matches(TAB_NEW) || editorPaneId) {
        e.preventDefault()
        openNewTabPage(
          workspaceStore.getState().workspaceId,
          editorPaneId ?? windowPaneStore.getState().activePaneId,
        )
        return
      }

      if (matches(TAB_NEW_TERMINAL)) {
        // A pane must hold a chat before a terminal opens in its editor view (spec §7.2);
        // ensurePaneChatThenOpen reuses the workspace's owning chat and never mints one.
        e.preventDefault()
        const targetPaneId = windowPaneStore.getState().activePaneId
        ensurePaneChatThenOpen(
          workspaceStore.getState().workspaceId,
          targetPaneId,
          (paneId, wsId) => {
            windowPaneStore
              .getState()
              .bufferActions.openContent({ type: 'terminal', workspaceId: wsId }, { paneId })
          },
        )
        return
      }

      if (matches(TAB_NEW_FILE)) {
        // Same chat-first rule as TAB_NEW_TERMINAL above.
        e.preventDefault()
        const targetPaneId = windowPaneStore.getState().activePaneId
        ensurePaneChatThenOpen(
          workspaceStore.getState().workspaceId,
          targetPaneId,
          (paneId, wsId) => void openUntitledDocument(wsId, paneId),
        )
        return
      }

      // The browser has no native file dialog; its own Cmd+O is left alone.
      if (matches(TAB_OPEN_FILE) && isTauri()) {
        e.preventDefault()
        void openDocumentFromDialog(windowPaneStore.getState().activePaneId)
        return
      }

      if (matches(AGENT_NEW_CHAT) || matches(AGENT_NEW_CHAT_TERMINAL)) {
        // Creates a chat on the first enabled provider and opens it as its own view (spec §8.4),
        // never into the active pane. The terminal variant lands this one chat on the CLI; a
        // provider whose terminal can't start a chat gets the ordinary create instead.
        const wantsTerminal = matches(AGENT_NEW_CHAT_TERMINAL)
        e.preventDefault()
        const state = workspaceStore.getState()
        const provider = useAgentProvidersStore.getState().providers.find((p) => p.enabled)
        if (!provider) return
        // The surface rides on the create because it picks which face the daemon forks; the
        // plain chord names none, so createSurfaceFor falls back to the user's default surface.
        const surface = createSurfaceFor(
          provider,
          wantsTerminal && providerCanStartOnTerminal(provider) ? 'terminal' : undefined,
        )
        // Not drawn as an operation row: the chat opens in its pane as soon as it exists.
        createChat(
          crypto.randomUUID(),
          crypto.randomUUID(),
          state.workspaceId,
          provider.id,
          '',
          surface,
        )
          .then((chatId) => {
            if (surface === 'terminal') {
              presetChatLandingPresentation(chatId, 'terminal')
            }
            // A brand-new chat has no runner yet — null until it spawns one.
            windowPaneStore
              .getState()
              .paneActions.openChat(chatId, { workspaceId: state.workspaceId })
          })
          .catch((err: unknown) => toastSpawnFailure(err, provider.displayName, 'start'))
        return
      }

      if (matches(TAB_REOPEN_CLOSED)) {
        e.preventDefault()
        windowPaneStore.getState().bufferActions.reopenLastClosedBuffer()
        return
      }

      if (matches(TAB_CLOSE)) {
        // Always claimed so it never reaches the OS window-close. Like the tab ×: remove the tab
        // from its pane first so a neighbour activates (closeBuffer alone leaves a dangling
        // activeEditorTabId), and prompt before discarding a dirty buffer.
        e.preventDefault()
        const state = windowPaneStore.getState()
        const paneId = state.activePaneId
        const pane = state.panes[paneId]
        if (!pane) return
        // With no tabs, ⌘W dismisses this split; in the last pane of its own tree it's a no-op.
        // Count within paneId's tree: `state.panes` always also holds the other tree's root,
        // so a raw pane count would close (and brick) the sole remaining pane.
        if (pane.editorTabIds.length === 0) {
          const scope = getPaneScopeForPaneId(
            showingLayout(state),
            state.bottomLayout,
            state.panes,
            paneId,
          )
          if (scope.length > 1) {
            state.paneActions.closePane(paneId)
          }
          return
        }
        const tabId = pane.activeEditorTabId
        if (!tabId) return
        const buf = state.buffers.find((b) => b.id === tabId)
        if (buf && buf.type === 'editor' && buf.isDirty) {
          state.bufferActions.setPendingClose({ type: 'single', bufferId: tabId })
          return
        }
        state.paneActions.removeEditorTabFromPane(paneId, tabId)
        state.bufferActions.closeBuffer(tabId)
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
          windowPaneStore.getState().paneActions.navigateToPane(direction)
          return
        }
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [workspaceStore, chordMap])
}
