import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { openWorkspaceFile } = vi.hoisted(() => ({ openWorkspaceFile: vi.fn(async () => {}) }))
vi.mock('@/features/files/lib/file-tree-handlers', () => ({ openWorkspaceFile }))
vi.mock('@/features/file-system/utils/file-system-dropped-paths', () => ({
  extractDroppedFilePaths: () => ['src/a.ts'],
}))
vi.mock('@/features/file-system/lib/tauri-file-drop', () => ({ useTauriFileDrop: vi.fn() }))

import { usePaneDropHandlers } from '@/features/panes/hooks/use-pane-drop-handlers'
import { resetWindowPaneStoreForTests } from '@/features/panes/stores/window-pane-store'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'

// What lands on a pane belongs to the pane's own workspace, which is not necessarily
// the one in view (a drop onto a non-active pane is the ordinary case).
describe('usePaneDropHandlers', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetWindowPaneStoreForTests()
    setActiveWorkspaceId('ws-in-view')
  })

  function render() {
    const container = document.createElement('div')
    return renderHook(() => usePaneDropHandlers(ROOT_PANE_ID, 'ws-pane', { current: container }))
  }

  it('opens files dropped from the OS in the pane’s workspace', async () => {
    const { result } = render()

    await act(async () => {
      await result.current.handleDrop({
        preventDefault: vi.fn(),
        stopPropagation: vi.fn(),
        dataTransfer: { types: ['Files'] },
      } as unknown as React.DragEvent)
    })

    expect(openWorkspaceFile).toHaveBeenCalledExactlyOnceWith('ws-pane', 'src/a.ts', {
      paneId: ROOT_PANE_ID,
    })
  })
})
