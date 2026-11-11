import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { openDocumentsAtPaths, useTauriFileDrop } = vi.hoisted(() => ({
  openDocumentsAtPaths: vi.fn(async () => {}),
  useTauriFileDrop: vi.fn(),
}))
vi.mock('@/features/editor/documents/open-document', () => ({ openDocumentsAtPaths }))
vi.mock('@/features/file-system/lib/tauri-file-drop', () => ({ useTauriFileDrop }))

import { usePaneDropHandlers } from '@/features/panes/hooks/use-pane-drop-handlers'
import { resetWindowPaneStoreForTests } from '@/features/panes/stores/window-pane-store'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'

// An OS drop carries host paths, which name their own files: they open into the pane they
// landed on, whichever workspace is in view.
describe('usePaneDropHandlers', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetWindowPaneStoreForTests()
    setActiveWorkspaceId('ws-in-view')
  })

  it('opens files dropped from the OS into the pane they landed on', async () => {
    const container = document.createElement('div')
    renderHook(() => usePaneDropHandlers(ROOT_PANE_ID, { current: container }))
    const [, onDrop] = useTauriFileDrop.mock.calls.at(-1) as [unknown, (paths: string[]) => void]

    await act(async () => onDrop(['/disk/src/a.ts']))

    expect(openDocumentsAtPaths).toHaveBeenCalledExactlyOnceWith(['/disk/src/a.ts'], ROOT_PANE_ID)
  })
})
