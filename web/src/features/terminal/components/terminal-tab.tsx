import { useCallback } from 'react'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { XtermTerminal } from './terminal'

interface TerminalTabProps {
  sessionId: string
  bufferId: string
  paneId?: string
  /**
   * The workspace THIS TERMINAL's buffer belongs to (TerminalContent.
   * workspaceId), NOT the ambient WorkspaceStoreContext. WorkspaceHost keeps
   * every WorkspaceView it retains mounted at once for keep-alive, each under
   * its OWN ambient context — a shell tab that reads the ambient workspace
   * instead of its own buffer's would resolve a hidden copy's connection
   * (doReconnect/initializeTerminal → terminalsBaseForWorkspace) against
   * whichever workspace happens to be active, not the one it actually belongs
   * to, and on a transport drop that can spawn a fresh PTY in the WRONG
   * worktree (see agent-chat-pane.tsx's `known` gate for the same shape of
   * bug). The buffer's own field is the one thing that can't be ambiently
   * wrong, the same way agent-terminal-surface.tsx threads chatId explicitly
   * instead of reading it off context.
   */
  workspaceId: string
  initialCommand?: string
  workingDirectory?: string
  remoteConnectionId?: string
  isActive?: boolean
  isVisible?: boolean
}

// Renders XtermTerminal directly — no portal indirection. The TerminalHost
// portal mechanism (TerminalSlot → XtermPortal) is reserved for the bottom
// panel terminal system where PTY sessions must survive pane rearrangements.
// Workspace pane terminals don't have that constraint yet.
export function TerminalTab({
  sessionId,
  bufferId,
  paneId,
  workspaceId,
  initialCommand,
  workingDirectory,
  remoteConnectionId,
  isActive = true,
  isVisible = true,
}: TerminalTabProps) {
  const handleTerminalExit = useCallback(() => {
    // The PTY is already gone, so this buffer must be torn down regardless of
    // how many panes still list it — closeBuffer only does that once NO pane
    // references the id any more (see its own doc comment: a pane that still
    // holds it reads as a SIBLING still showing a live split, which this is
    // not). Strip every pane's membership first.
    const state = windowPaneStore.getState()
    for (const pane of Object.values(state.panes)) {
      if (pane.editorTabIds.includes(bufferId)) {
        state.paneActions.removeEditorTabFromPane(pane.id, bufferId)
      }
    }
    state.bufferActions.closeBuffer(bufferId)
  }, [bufferId])

  const handleActivate = useCallback(() => {
    // I8 (Task 26 fix round 1): addBufferToPane/activatePaneBuffer have not
    // existed on PaneActions since Task 1's editorTabIds rename — this threw
    // on every click into a pane terminal. The tab is already open (it is
    // mounted, rendering this very terminal); activate it in place —
    // resolving the holding pane when the caller doesn't already know it.
    const { paneActions } = windowPaneStore.getState()
    const targetPaneId = paneId ?? paneActions.getPaneByEditorTabId(bufferId)?.id
    if (targetPaneId) {
      paneActions.activateEditorTabInPane(targetPaneId, bufferId)
      // The mousedown capture this drives exists to "activate the pane" (see
      // the comment on the container below) — not just the tab within it.
      paneActions.setActivePane(targetPaneId)
    }
  }, [bufferId, paneId])

  return (
    // onMouseDownCapture: xterm canvas events don't bubble through React, so we
    // capture the native mousedown here to activate the pane without fighting
    // the portal boundary.
    <div
      className="flex h-full w-full flex-col overflow-hidden"
      onMouseDownCapture={handleActivate}
    >
      <XtermTerminal
        sessionId={sessionId}
        workspaceId={workspaceId}
        isActive={isActive}
        isVisible={isVisible}
        onTerminalExit={handleTerminalExit}
        initialCommand={initialCommand}
        workingDirectory={workingDirectory}
        remoteConnectionId={remoteConnectionId}
      />
    </div>
  )
}
