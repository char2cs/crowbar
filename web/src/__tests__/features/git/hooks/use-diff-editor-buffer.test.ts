import { renderHook } from '@testing-library/react'
import { createElement, type ReactNode } from 'react'
import { describe, expect, it } from 'vitest'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { useDiffEditorBuffer } from '@/features/git/hooks/use-diff-editor-buffer'

function makeWrapper(store: ReturnType<typeof createWorkspaceStore>) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return createElement(WorkspaceStoreContext.Provider, { value: store }, children)
  }
}

function diffEditorBuffers(store: ReturnType<typeof createWorkspaceStore>) {
  return store.getState().buffers.filter((b) => b.id.startsWith('diff_editor_'))
}

// The split-diff left/right editors only need a registered buffer while split
// view is active (Task 9 / P2d) — `enabled` lets a caller skip the
// setState + Monaco-model churn entirely rather than registering a buffer that
// is never shown.
describe('useDiffEditorBuffer enabled gating', () => {
  it('registers a buffer by default (enabled omitted)', () => {
    const store = createWorkspaceStore('w1')
    renderHook(() => useDiffEditorBuffer({ cacheKey: 'file1', content: 'hello', name: 'file1' }), {
      wrapper: makeWrapper(store),
    })

    expect(diffEditorBuffers(store)).toHaveLength(1)
  })

  it('registers no buffer when enabled is false', () => {
    const store = createWorkspaceStore('w1')
    renderHook(
      () =>
        useDiffEditorBuffer({
          cacheKey: 'file1_left',
          content: 'left content',
          name: 'file1 (left)',
          enabled: false,
        }),
      { wrapper: makeWrapper(store) },
    )

    expect(diffEditorBuffers(store)).toHaveLength(0)
  })

  it('registers the buffer the moment enabled flips true mid-session', () => {
    const store = createWorkspaceStore('w1')
    const { rerender } = renderHook(
      ({ enabled }: { enabled: boolean }) =>
        useDiffEditorBuffer({
          cacheKey: 'file1_left',
          content: 'left content',
          name: 'file1 (left)',
          enabled,
        }),
      { wrapper: makeWrapper(store), initialProps: { enabled: false } },
    )
    expect(diffEditorBuffers(store)).toHaveLength(0)

    rerender({ enabled: true })
    expect(diffEditorBuffers(store)).toHaveLength(1)
  })

  it('unregisters the buffer the moment enabled flips back to false', () => {
    const store = createWorkspaceStore('w1')
    const { rerender } = renderHook(
      ({ enabled }: { enabled: boolean }) =>
        useDiffEditorBuffer({
          cacheKey: 'file1_left',
          content: 'left content',
          name: 'file1 (left)',
          enabled,
        }),
      { wrapper: makeWrapper(store), initialProps: { enabled: true } },
    )
    expect(diffEditorBuffers(store)).toHaveLength(1)

    rerender({ enabled: false })
    expect(diffEditorBuffers(store)).toHaveLength(0)
  })

  it('removes the buffer on unmount regardless of enabled', () => {
    const store = createWorkspaceStore('w1')
    const { unmount } = renderHook(
      () => useDiffEditorBuffer({ cacheKey: 'file1', content: 'hello', name: 'file1' }),
      { wrapper: makeWrapper(store) },
    )
    expect(diffEditorBuffers(store)).toHaveLength(1)

    unmount()
    expect(diffEditorBuffers(store)).toHaveLength(0)
  })
})
