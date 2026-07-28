import { beforeEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import { useGitDiffHandlers } from '@/features/git/hooks/use-git-diff-handlers'

const openContent = vi.fn()

vi.mock('@/features/workspace/stores/workspace-store-ref', () => ({
  getActiveWorkspaceStoreRef: () => ({
    getState: () => ({ bufferActions: { openContent } }),
  }),
}))

vi.mock('@/components/ui/primitive-dialog-service', () => ({
  primitiveAlert: vi.fn(),
}))

beforeEach(() => {
  openContent.mockClear()
})

describe('handleViewCommitDiff', () => {
  it('opens a tab carrying the SHA, not the diff', async () => {
    // The point of the migration: a commit tab must cost the same whether the
    // commit changed three lines or a million. It used to fetch the entire
    // commit diff up front and hand the payload to the tab.
    const { result } = renderHook(() =>
      useGitDiffHandlers({ activeRepoPath: 'ws-1', onFileSelect: vi.fn() }),
    )

    await act(async () => {
      await result.current.handleViewCommitDiff('a6ec1f7f4e0a8b930104e534172de4a5171ced5d')
    })

    expect(openContent).toHaveBeenCalledWith({
      type: 'commitDiff',
      wsId: 'ws-1',
      sha: 'a6ec1f7f4e0a8b930104e534172de4a5171ced5d',
      name: 'Commit a6ec1f7',
    })
    // No payload keys at all — nothing was fetched to put in one.
    const spec = openContent.mock.calls[0][0]
    expect(spec).not.toHaveProperty('diffData')
    expect(spec).not.toHaveProperty('content')
  })

  it('does nothing without a workspace to open into', async () => {
    const { result } = renderHook(() =>
      useGitDiffHandlers({ activeRepoPath: null, onFileSelect: vi.fn() }),
    )

    await act(async () => {
      await result.current.handleViewCommitDiff('abc1234')
    })

    expect(openContent).not.toHaveBeenCalled()
  })
})
