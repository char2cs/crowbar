import { useCallback, useRef } from 'react'
import { IS_MOBILE } from '@/utils/platform'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { useTerminalStore } from '../stores/terminal-store'
import { XtermTerminal, type TerminalFocusHandle } from './terminal'
import { TerminalKeyBar } from './terminal-key-bar'

interface TerminalTabProps {
  sessionId: string
  bufferId: string
  paneId?: string
  /** The buffer's own workspace, never the ambient context's: with several workspaces mounted,
   *  the ambient one could reconnect or spawn this tab's PTY in the wrong worktree. */
  workspaceId: string
  initialCommand?: string
  workingDirectory?: string
  isActive?: boolean
  isVisible?: boolean
}

// Renders XtermTerminal directly — no portal indirection: the pane container
// keeps a terminal buffer's view mounted (visibility:hidden) across layout
// changes, so its xterm and transport survive them.
export function TerminalTab({
  sessionId,
  bufferId,
  paneId,
  workspaceId,
  initialCommand,
  workingDirectory,
  isActive = true,
  isVisible = true,
}: TerminalTabProps) {
  const terminal = useRef<TerminalFocusHandle>(null)
  const handleTerminalExit = useCallback(() => {
    // Forget the exited PTY first, so closing the buffer does not DELETE a
    // session the daemon has already reaped (a 404 on every `exit`).
    useTerminalStore.getState().removeSession(sessionId)
    // The PTY is gone, but closeBuffer tears a buffer down only once no pane references it (a
    // holder reads as a live split sibling), so strip every pane's membership first.
    const state = windowPaneStore.getState()
    for (const pane of Object.values(state.panes)) {
      if (pane.editorTabIds.includes(bufferId)) {
        state.paneActions.removeEditorTabFromPane(pane.id, bufferId)
      }
    }
    state.bufferActions.closeBuffer(bufferId)
  }, [bufferId, sessionId])

  const handleActivate = useCallback(() => {
    // The tab is already open (it renders this terminal): activate it in place, resolving the
    // holding pane when the caller doesn't know it.
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
        ref={terminal}
        sessionId={sessionId}
        workspaceId={workspaceId}
        paneId={paneId}
        isActive={isActive}
        isVisible={isVisible}
        onTerminalExit={handleTerminalExit}
        initialCommand={initialCommand}
        workingDirectory={workingDirectory}
      />
      {/* The keys a phone keyboard lacks. */}
      {IS_MOBILE && <TerminalKeyBar onSend={(bytes) => terminal.current?.send(bytes)} />}
    </div>
  )
}
