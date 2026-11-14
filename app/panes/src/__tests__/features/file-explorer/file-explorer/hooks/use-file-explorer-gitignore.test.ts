import { renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { readDocument } = vi.hoisted(() => ({ readDocument: vi.fn() }))
vi.mock('@/lib/api/documents', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/documents')>()),
  readDocument,
}))

import { useFileExplorerGitignore } from '@/features/file-explorer/file-explorer/hooks/use-file-explorer-gitignore'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { workspaceRef } from '@/lib/api/documents'

const references = [{ path: '.gitignore', directoryPath: '' }]

// The tree shows one workspace; its ignore rules come from THAT workspace's files,
// not from whichever other workspace happens to be in view.
describe('useFileExplorerGitignore', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    readDocument.mockImplementation(async (ref: unknown) => ({ ref, text: 'dist/\n' }))
    setActiveWorkspaceId('ws-in-view')
  })

  it('reads the ignore files of the workspace the tree shows', async () => {
    const { result } = renderHook(() =>
      useFileExplorerGitignore('ws-a', 'ws-a', references, () => 'ws-a'),
    )

    await waitFor(() => expect(result.current.isGitIgnored('dist', true)).toBe(true))
    expect(readDocument).toHaveBeenCalledExactlyOnceWith(workspaceRef('ws-a', '.gitignore'))
  })

  it('reads nothing when the tree shows no workspace', async () => {
    renderHook(() => useFileExplorerGitignore(null, 'ws-a', references, () => 'ws-a'))

    await Promise.resolve()
    expect(readDocument).not.toHaveBeenCalled()
  })
})
