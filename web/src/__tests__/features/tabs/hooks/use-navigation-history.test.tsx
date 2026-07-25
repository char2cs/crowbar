import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import { useNavigationHistory } from '@/features/tabs/hooks/use-navigation-history'
import { useJumpListStore } from '@/features/editor/stores/jump-list-store'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { setActiveWorkspaceStoreRef } from '@/features/workspace/stores/workspace-store-ref'

vi.mock('@/features/editor/stores/state-store', () => ({
  useEditorStateStore: {
    getState: () => ({
      cursorPosition: { line: 1, column: 1, offset: 0 },
      scrollTop: 0,
      scrollLeft: 0,
    }),
  },
}))

function openFile(store: ReturnType<typeof createWorkspaceStore>, path: string) {
  return store.getState().bufferActions.openContent({
    type: 'editor',
    path,
    name: path.split('/').pop()!,
    content: path,
  })
}

beforeEach(() => {
  useJumpListStore.getState().actions.clear()
})

afterEach(() => {
  setActiveWorkspaceStoreRef(null)
})

describe('useNavigationHistory', () => {
  it('records a jump-list entry when the user switches files inside one workspace', () => {
    const ws = createWorkspaceStore('w1')
    const first = openFile(ws, '/src/a.ts')
    setActiveWorkspaceStoreRef(ws)

    renderHook(() => useNavigationHistory())

    act(() => {
      openFile(ws, '/src/b.ts')
    })

    const entries = useJumpListStore.getState().entries
    expect(entries).toHaveLength(1)
    expect(entries[0].bufferId).toBe(first)
    expect(entries[0].filePath).toBe('/src/a.ts')
  })

  it('stamps the recording workspace onto every entry', () => {
    const ws = createWorkspaceStore('w1')
    openFile(ws, '/src/a.ts')
    setActiveWorkspaceStoreRef(ws)
    renderHook(() => useNavigationHistory())

    act(() => {
      openFile(ws, '/src/b.ts')
    })

    // Without this the (process-global) jump list cannot tell which checkout a
    // workspace-relative path belongs to.
    expect(useJumpListStore.getState().entries[0].workspaceId).toBe('w1')
  })

  /**
   * The failure this exists for: switching workspaces re-runs the effect once
   * with the INCOMING workspace's activeBufferId while the origin ref still
   * holds the OUTGOING workspace's buffer. That pushes a cross-workspace entry
   * into a process-global list whose paths are workspace-relative — Back then
   * silently opens the sibling worktree's file of the same name.
   *
   * A workspace switch is not a navigation. Nothing should be recorded.
   */
  it('records nothing when the ACTIVE WORKSPACE changed rather than the file', () => {
    const a = createWorkspaceStore('w1')
    openFile(a, '/src/app.ts')
    setActiveWorkspaceStoreRef(a)
    renderHook(() => useNavigationHistory())

    const b = createWorkspaceStore('w2')
    openFile(b, '/src/app.ts')
    act(() => {
      setActiveWorkspaceStoreRef(b)
    })

    expect(useJumpListStore.getState().entries).toEqual([])
  })

  it('resumes recording normally inside the workspace it switched to', () => {
    const a = createWorkspaceStore('w1')
    openFile(a, '/src/app.ts')
    setActiveWorkspaceStoreRef(a)
    renderHook(() => useNavigationHistory())

    const b = createWorkspaceStore('w2')
    const bFirst = openFile(b, '/src/app.ts')
    act(() => {
      setActiveWorkspaceStoreRef(b)
    })
    act(() => {
      openFile(b, '/src/other.ts')
    })

    const entries = useJumpListStore.getState().entries
    expect(entries).toHaveLength(1)
    expect(entries[0].bufferId).toBe(bFirst)
    expect(entries[0].workspaceId).toBe('w2')
  })
})
