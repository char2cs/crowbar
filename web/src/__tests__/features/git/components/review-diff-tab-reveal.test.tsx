import { useLayoutEffect } from 'react'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { act, render } from '@testing-library/react'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { ReviewDiffTab } from '@/features/git/components/review-diff-tab'

// Captures what the surface was asked to reveal. The real ReviewCodeView is
// lazy and drives a web component, neither of which a jsdom test can host —
// what is under test here is the WIRE from the sidebar's click to the handle,
// which is exactly what broke.
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
    // Attached in a layout effect with a STABLE identity, mirroring the real
    // component's useImperativeHandle. Attaching during render would set state
    // in another component's render pass, and a fresh object each render would
    // re-attach, re-render and spin forever.
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

// The surface is behind React.lazy + Suspense, so the handle does not exist on
// the first commit — flush the lazy resolution before asking for a reveal, the
// same way a real click happens long after the pane has painted.
async function renderTab(store: ReturnType<typeof createWorkspaceStore>, commit?: string) {
  const view = render(
    <WorkspaceStoreContext.Provider value={store}>
      <ReviewDiffTab onRetry={vi.fn()} commit={commit} />
    </WorkspaceStoreContext.Provider>,
  )
  await act(async () => {
    await Promise.resolve()
  })
  return view
}

beforeEach(() => {
  revealFile.mockClear()
})

describe('ReviewDiffTab — reveal a file from the sidebar', () => {
  it('scrolls the surface to the path the changed-files tree asked for', async () => {
    // The regression: this used to resolve a `path:index` key against a
    // whole-diff cache the surface stopped loading, so the click opened the tab
    // and did nothing else.
    const store = createWorkspaceStore('w1')
    await renderTab(store)

    act(() => store.getState().revealBranchReviewFile('src/a.ts'))

    expect(revealFile).toHaveBeenCalledWith('src/a.ts')
  })

  it('reveals again when the SAME file is clicked twice', async () => {
    // Why the request carries a nonce: the path compares equal to itself, so an
    // effect keyed on the path alone would fire once and never again.
    const store = createWorkspaceStore('w1')
    await renderTab(store)

    act(() => store.getState().revealBranchReviewFile('src/a.ts'))
    act(() => store.getState().revealBranchReviewFile('src/a.ts'))

    expect(revealFile).toHaveBeenCalledTimes(2)
  })

  it('does not hijack a COMMIT tab', async () => {
    // A commit tab shows a different diff; a request raised by the branch
    // review's file list must not scroll it to a path it may not even contain.
    const store = createWorkspaceStore('w1')
    await renderTab(store, 'abc1234')

    act(() => store.getState().revealBranchReviewFile('src/a.ts'))

    expect(revealFile).not.toHaveBeenCalled()
  })
})

describe('ReviewDiffTab — a request raised before the surface exists', () => {
  it('honours a reveal that was queued while the tab was still closed', async () => {
    // The path that actually broke in the app: clicking a file in the sidebar
    // OPENS this tab, so the request is raised before the lazy surface has
    // mounted and there is no handle to call. Firing blind dropped it and the
    // pane opened at the top of the diff.
    const store = createWorkspaceStore('w1')
    act(() => store.getState().revealBranchReviewFile('src/a.ts'))

    await renderTab(store)

    expect(revealFile).toHaveBeenCalledWith('src/a.ts')
  })
})
