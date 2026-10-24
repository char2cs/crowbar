import { useCallback } from 'react'
import { useJumpListStore } from '@/features/editor/stores/jump-list-store'
import { useEditorStateStore } from '@/features/editor/stores/state-store'
import { navigateToJumpEntry } from '@/features/editor/utils/jump-navigation'
import { documentRefOf } from '@/features/editor/documents/document-ref'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { isEditorContent } from '@/features/panes/types/pane-content'

export function useJumpNavigation() {
  const jumpListActions = useJumpListStore.use.actions()
  useJumpListStore.use.entries()
  useJumpListStore.use.currentIndex()

  const canGoBack = jumpListActions.canGoBack()
  const canGoForward = jumpListActions.canGoForward()

  const handleJumpBack = useCallback(async () => {
    const paneState = windowPaneStore.getState()
    const editorState = useEditorStateStore.getState()
    const currentActiveBufferId = paneState.panes[paneState.activePaneId]?.activeEditorTabId ?? null
    const currentActiveBuffer = currentActiveBufferId
      ? paneState.buffers.find((b) => b.id === currentActiveBufferId)
      : undefined

    const currentPosition =
      currentActiveBufferId && currentActiveBuffer && isEditorContent(currentActiveBuffer)
        ? {
            bufferId: currentActiveBufferId,
            doc: documentRefOf(currentActiveBuffer),
            line: editorState.cursorPosition.line,
            column: editorState.cursorPosition.column,
            offset: editorState.cursorPosition.offset,
            scrollTop: 0,
            scrollLeft: 0,
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
