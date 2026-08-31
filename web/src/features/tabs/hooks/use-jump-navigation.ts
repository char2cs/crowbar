import { useCallback } from 'react'
import { useJumpListStore } from '@/features/editor/stores/jump-list-store'
import { useEditorStateStore } from '@/features/editor/stores/state-store'
import { navigateToJumpEntry } from '@/features/editor/utils/jump-navigation'
import { getActiveWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'

export function useJumpNavigation() {
  const jumpListActions = useJumpListStore.use.actions()
  useJumpListStore.use.entries()
  useJumpListStore.use.currentIndex()

  const canGoBack = jumpListActions.canGoBack()
  const canGoForward = jumpListActions.canGoForward()

  const handleJumpBack = useCallback(async () => {
    const store = getActiveWorkspaceStore()
    if (!store) return
    const wsState = store.getState()
    const paneState = windowPaneStore.getState()
    const editorState = useEditorStateStore.getState()
    const currentActiveBufferId =
      paneState.panes[paneState.activePaneId]?.activeEditorTabId ?? null
    const currentActiveBuffer = currentActiveBufferId
      ? paneState.buffers.find((b) => b.id === currentActiveBufferId)
      : undefined

    const currentPosition =
      currentActiveBufferId && currentActiveBuffer?.path
        ? {
            bufferId: currentActiveBufferId,
            // The forward-stack entry this synthesises is a position in THIS
            // workspace; without the stamp it would be unresolvable later.
            workspaceId: wsState.workspaceId,
            filePath: currentActiveBuffer.path,
            line: editorState.cursorPosition.line,
            column: editorState.cursorPosition.column,
            offset: editorState.cursorPosition.offset,
            scrollTop: editorState.scrollTop,
            scrollLeft: editorState.scrollLeft,
          }
        : undefined

    const entry = jumpListActions.goBack(currentPosition)
    if (entry) {
      await navigateToJumpEntry(entry)
    }
  }, [jumpListActions])

  const handleJumpForward = useCallback(async () => {
    const entry = jumpListActions.goForward()
    if (entry) {
      await navigateToJumpEntry(entry)
    }
  }, [jumpListActions])

  return { canGoBack, canGoForward, handleJumpBack, handleJumpForward }
}
