import { createElement, type ReactNode } from 'react'
import { act, renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { useDiffData } from '@/features/git/hooks/use-git-diff-data'

function seedStore() {
  const store = createWorkspaceStore('w1')
  const diffJson = JSON.stringify({
    file_path: 'src/app.ts',
    old_path: 'src/app.ts',
    new_path: 'src/app.ts',
    lines: [],
  })
  // Open the UNRELATED editor buffer first, then the diff buffer — openContent
  // activates the last-opened one, so the diff buffer ends up active (the one
  // useDiffData reads) while the editor is the churn target.
  store.getState().bufferActions.openContent({
    type: 'editor',
    path: '/project/other.ts',
    name: 'other.ts',
    content: 'a',
  })
  store.getState().bufferActions.openContent({
    type: 'diff',
    path: 'diff://unstaged/src%2Fapp.ts',
    name: 'app.ts',
    content: diffJson,
  })
  return store
}

function wrapperFor(store: ReturnType<typeof createWorkspaceStore>) {
  return ({ children }: { children: ReactNode }) =>
    createElement(WorkspaceStoreContext.Provider, { value: store }, children)
}

describe('useDiffData subscription isolation', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  it('parses the active diff buffer content once and ignores unrelated buffer churn', () => {
    const store = seedStore()

    let renders = 0
    const { result } = renderHook(
      () => {
        renders += 1
        return useDiffData()
      },
      { wrapper: wrapperFor(store) },
    )

    // The JSON content parsed into a GitDiff (the `file_path` branch).
    expect(result.current.rawDiffData).toMatchObject({ file_path: 'src/app.ts' })
    expect(result.current.diff?.file_path).toBe('src/app.ts')

    const rawRef = result.current.rawDiffData
    const rendersAfterMount = renders

    // Churn a DIFFERENT buffer's content (an editor keystroke). immer keeps the
    // active diff buffer's reference identical, so the id-scoped subscription
    // must not fire and the memoized parse must not re-run.
    act(() => {
      store.setState((s) => ({
        ...s,
        buffers: s.buffers.map((b) => (b.type === 'editor' ? { ...b, content: 'aa' } : b)),
      }))
    })

    expect(renders).toBe(rendersAfterMount)
    expect(result.current.rawDiffData).toBe(rawRef)
  })
})
