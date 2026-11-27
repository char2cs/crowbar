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
  workspaceId: string
  initialCommand?: string
  workingDirectory?: string
  isActive?: boolean
  isVisible?: boolean
}

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
    useTerminalStore.getState().removeSession(sessionId)
    const state = windowPaneStore.getState()
    for (const pane of Object.values(state.panes)) {
      if (pane.editorTabIds.includes(bufferId)) {
        state.paneActions.removeEditorTabFromPane(pane.id, bufferId)
      }
    }
    state.bufferActions.closeBuffer(bufferId)
  }, [bufferId, sessionId])

  const handleActivate = useCallback(() => {
    const { paneActions } = windowPaneStore.getState()
    const targetPaneId = paneId ?? paneActions.getPaneByEditorTabId(bufferId)?.id
    if (targetPaneId) {
      paneActions.activateEditorTabInPane(targetPaneId, bufferId)
      paneActions.setActivePane(targetPaneId)
    }
  }, [bufferId, paneId])

  return (
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
      {IS_MOBILE && <TerminalKeyBar onSend={(bytes) => terminal.current?.send(bytes)} />}
    </div>
  )
}
