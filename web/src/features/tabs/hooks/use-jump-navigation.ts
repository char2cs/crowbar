import { useCallback } from 'react'
import { useJumpListStore } from '@/features/editor/stores/jump-list-store'
import { useEditorStateStore } from '@/features/editor/stores/state-store'
import { navigateToJumpEntry } from '@/features/editor/utils/jump-navigation'
import { getActiveWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'

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
    const editorState = useEditorStateStore.getState()
    const currentActiveBufferId = wsState.panes[wsState.activePaneId]?.activeBufferId ?? null
    const currentActiveBuffer = currentActiveBufferId
      ? wsState.buffers.find((b) => b.id === currentActiveBufferId)
      : undefined

    const currentPosition =
      currentActiveBufferId && currentActiveBuffer?.path
        ? {
            bufferId: currentActiveBufferId,
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
