import { useLayoutEffect } from 'react'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { act, render } from '@testing-library/react'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { ReviewDiffTab, type LineRequest } from '@/features/git/components/review-diff-tab'

const revealFile = vi.fn()
const revealLine = vi.fn()
const HANDLE = { revealFile, revealLine }

vi.mock('@/features/git/components/diff/review-code-view', () => ({
  ReviewCodeView: ({
    surfaceRef,
  }: {
    surfaceRef?:
      | { current: { revealFile: (p: string) => void; revealLine: () => void } | null }
      | ((h: { revealFile: (p: string) => void; revealLine: () => void } | null) => void)
  }) => {
    useLayoutEffect(() => {
      if (typeof surfaceRef === 'function') surfaceRef(HANDLE)
      else if (surfaceRef) surfaceRef.current = HANDLE
    }, [surfaceRef])
    return null
  },
}))

vi.mock('@/features/git/hooks/use-review-files-summary', () => ({
  useReviewFilesSummary: () => ({
    files: [
      { file_path: 'src/a.ts', is_new: false, is_deleted: false, is_renamed: false, lines: [] },
    ],
    loaded: true,
  }),
}))

vi.mock('@/features/git/hooks/use-review-outline', () => ({
  useReviewOutline: () => ({ outline: [], loaded: true }),
}))

async function renderTab(
  store: ReturnType<typeof createWorkspaceStore>,
  commit?: string,
  lineRequest?: LineRequest,
) {
  const view = render(
    <WorkspaceStoreContext.Provider value={store}>
      <ReviewDiffTab
        onRetry={vi.fn()}
        wsId={store.getState().workspaceId}
        commit={commit}
        lineRequest={lineRequest}
      />
    </WorkspaceStoreContext.Provider>,
  )
  await act(async () => {
    await Promise.resolve()
  })
  return view
}

beforeEach(() => {
  revealFile.mockClear()
  revealLine.mockClear()
})

describe('ReviewDiffTab — reveal a file from the sidebar', () => {
  it('scrolls the surface to the path the changed-files tree asked for', async () => {
    const store = createWorkspaceStore('w1')
    await renderTab(store)

    act(() => store.getState().revealBranchReviewFile('src/a.ts'))

    expect(revealFile).toHaveBeenCalledWith('src/a.ts')
  })

  it('reveals again when the SAME file is clicked twice', async () => {
    const store = createWorkspaceStore('w1')
    await renderTab(store)

    act(() => store.getState().revealBranchReviewFile('src/a.ts'))
    act(() => store.getState().revealBranchReviewFile('src/a.ts'))

    expect(revealFile).toHaveBeenCalledTimes(2)
  })

  it('does not hijack a COMMIT tab', async () => {
    const store = createWorkspaceStore('w1')
    await renderTab(store, 'abc1234')

    act(() => store.getState().revealBranchReviewFile('src/a.ts'))

    expect(revealFile).not.toHaveBeenCalled()
  })
})

describe('ReviewDiffTab — a request raised before the surface exists', () => {
  it('honours a reveal that was queued while the tab was still closed', async () => {
    const store = createWorkspaceStore('w1')
    act(() => store.getState().revealBranchReviewFile('src/a.ts'))

    await renderTab(store)

    expect(revealFile).toHaveBeenCalledWith('src/a.ts')
  })
})

describe('ReviewDiffTab — jump to a thread line', () => {
  it('reveals the requested line once the surface mounts, and only once per request', async () => {
    const store = createWorkspaceStore('w1')
    const request: LineRequest = { path: 'src/a.ts', lineNumber: 7, side: 'old', nonce: 1 }

    const view = await renderTab(store, undefined, request)
    expect(revealLine).toHaveBeenCalledExactlyOnceWith('src/a.ts', 7, 'old')

    view.rerender(
      <WorkspaceStoreContext.Provider value={store}>
        <ReviewDiffTab onRetry={vi.fn()} wsId="w1" lineRequest={{ ...request }} />
      </WorkspaceStoreContext.Provider>,
    )
    expect(revealLine).toHaveBeenCalledTimes(1)

    view.rerender(
      <WorkspaceStoreContext.Provider value={store}>
        <ReviewDiffTab onRetry={vi.fn()} wsId="w1" lineRequest={{ ...request, nonce: 2 }} />
      </WorkspaceStoreContext.Provider>,
    )
    expect(revealLine).toHaveBeenCalledTimes(2)
  })
})
