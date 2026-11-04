import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { readWorkspaceDirectory } = vi.hoisted(() => ({ readWorkspaceDirectory: vi.fn() }))
vi.mock('@/features/file-system/controllers/platform', () => ({ readWorkspaceDirectory }))

import { useFileExplorerDialogs } from '@/features/file-explorer/file-explorer/hooks/use-file-explorer-dialogs'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'

// "Open All" scans the workspace the tree shows, not whichever one is in view.
describe('useFileExplorerDialogs — Open All', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setActiveWorkspaceId('ws-in-view')
    readWorkspaceDirectory.mockResolvedValue([
      { name: 'a.ts', path: 'src/a.ts', is_dir: false, isDirectory: false, isFile: true },
    ])
  })

  function open(workspaceId: string | null) {
    const onFileSelect = vi.fn()
    const { result } = renderHook(() =>
      useFileExplorerDialogs({
        workspaceId,
        filteredFiles: [],
        isVisible: () => true,
        rootFolderPath: 'ws-a',
        onFileSelect,
      }),
    )
    return { result, onFileSelect }
  }

  it('scans and opens the files of the workspace the tree shows', async () => {
    const { result, onFileSelect } = open('ws-a')

    await act(async () => result.current.openAllFilesInDirectory('src'))

    expect(readWorkspaceDirectory).toHaveBeenCalledExactlyOnceWith('ws-a', 'src')
    expect(onFileSelect).toHaveBeenCalledWith('src/a.ts', false)
  })

  it('scans nothing when the tree shows no workspace', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    const { result } = open(null)

    await act(async () => result.current.openAllFilesInDirectory('src'))

    expect(readWorkspaceDirectory).not.toHaveBeenCalled()
    consoleError.mockRestore()
  })
})
