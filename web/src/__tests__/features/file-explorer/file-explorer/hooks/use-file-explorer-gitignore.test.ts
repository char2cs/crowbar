import { renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { readWorkspaceFile } = vi.hoisted(() => ({ readWorkspaceFile: vi.fn() }))
vi.mock('@/features/file-system/controllers/platform', () => ({ readWorkspaceFile }))

import { useFileExplorerGitignore } from '@/features/file-explorer/file-explorer/hooks/use-file-explorer-gitignore'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'

const references = [{ path: '.gitignore', directoryPath: '' }]

// The tree shows one workspace; its ignore rules come from THAT workspace's files,
// not from whichever other workspace happens to be in view.
describe('useFileExplorerGitignore', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    readWorkspaceFile.mockResolvedValue('dist/\n')
    setActiveWorkspaceId('ws-in-view')
  })

  it('reads the ignore files of the workspace the tree shows', async () => {
    const { result } = renderHook(() =>
      useFileExplorerGitignore('ws-a', 'ws-a', references, () => 'ws-a'),
    )

    await waitFor(() => expect(result.current.isGitIgnored('dist', true)).toBe(true))
    expect(readWorkspaceFile).toHaveBeenCalledExactlyOnceWith('ws-a', '.gitignore')
  })

  it('reads nothing when the tree shows no workspace', async () => {
    renderHook(() => useFileExplorerGitignore(null, 'ws-a', references, () => 'ws-a'))

    await Promise.resolve()
    expect(readWorkspaceFile).not.toHaveBeenCalled()
  })
})
