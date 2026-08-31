import { useCallback } from 'react'
import { useWorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { XtermTerminal } from './terminal'

interface TerminalTabProps {
  sessionId: string
  bufferId: string
  paneId?: string
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
  initialCommand,
  workingDirectory,
  remoteConnectionId,
  isActive = true,
  isVisible = true,
}: TerminalTabProps) {
  // The owning workspace — threaded into the terminal so connection resolution
  // targets THIS workspace even when it is a hidden keep-alive workspace.
  const workspaceId = useWorkspaceStoreContext((s) => s.workspaceId)

  const handleTerminalExit = useCallback(() => {
    windowPaneStore.getState().bufferActions.closeBuffer(bufferId)
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
