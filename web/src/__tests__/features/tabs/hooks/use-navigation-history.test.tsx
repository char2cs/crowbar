import { describe, it, expect, beforeEach, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import { useNavigationHistory } from '@/features/tabs/hooks/use-navigation-history'
import { useJumpListStore } from '@/features/editor/stores/jump-list-store'
import { windowPaneStore, resetWindowPaneStoreForTests } from '@/features/panes/stores/window-pane-store'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'

vi.mock('@/features/editor/stores/state-store', () => ({
  useEditorStateStore: {
    getState: () => ({
      cursorPosition: { line: 1, column: 1, offset: 0 },
      scrollTop: 0,
      scrollLeft: 0,
    }),
  },
}))

function openFile(workspaceId: string, path: string) {
  return windowPaneStore.getState().bufferActions.openContent({
    type: 'editor',
    path,
    name: path.split('/').pop()!,
    content: path,
    workspaceId,
  })
}

beforeEach(() => {
  useJumpListStore.getState().actions.clear()
  // Task 26: panes/buffers are window-level and never destroyed, so the
  // singleton persists across tests — reset it to a pristine, empty store
  // (mirroring createWindowPaneStore's own defaults) before each test.
  resetWindowPaneStoreForTests()
  setActiveWorkspaceId('w1')
})

describe('useNavigationHistory', () => {
  it('records a jump-list entry when the user switches files inside one workspace', () => {
    const first = openFile('w1', '/src/a.ts')

    renderHook(() => useNavigationHistory())

    act(() => {
      openFile('w1', '/src/b.ts')
    })

    const entries = useJumpListStore.getState().entries
    expect(entries).toHaveLength(1)
    expect(entries[0].bufferId).toBe(first)
    expect(entries[0].filePath).toBe('/src/a.ts')
  })

  it('stamps the recording workspace onto every entry', () => {
    openFile('w1', '/src/a.ts')
    renderHook(() => useNavigationHistory())

    act(() => {
      openFile('w1', '/src/b.ts')
    })

    // Without this the (process-global) jump list cannot tell which checkout a
    // workspace-relative path belongs to.
    expect(useJumpListStore.getState().entries[0].workspaceId).toBe('w1')
  })

  /**
   * The failure this exists for: two panes/buffers holding the SAME relative
   * path in different workspaces must never be confused for one navigation —
   * the jump list is a process-global singleton with workspace-relative
   * paths, so recording across the boundary puts a foreign entry into a
   * workspace's history. Back would then silently open the sibling
   * worktree's file of the same name.
   *
   * A workspace switch is not a navigation. Nothing should be recorded.
   */
  it('records nothing when the ACTIVE WORKSPACE changed rather than the file', () => {
    openFile('w1', '/src/app.ts')
    renderHook(() => useNavigationHistory())

    openFile('w2', '/src/app.ts')
    act(() => {
      setActiveWorkspaceId('w2')
      windowPaneStore.getState().paneActions.activateEditorTabInPane(
        windowPaneStore.getState().activePaneId,
        windowPaneStore
          .getState()
          .buffers.find((b) => b.workspaceId === 'w2' && b.path === '/src/app.ts')!.id,
      )
    })

    expect(useJumpListStore.getState().entries).toEqual([])
  })

  it('resumes recording normally inside the workspace it switched to', () => {
    openFile('w1', '/src/app.ts')
    renderHook(() => useNavigationHistory())

    const bFirst = openFile('w2', '/src/app.ts')
    act(() => {
      setActiveWorkspaceId('w2')
      windowPaneStore
        .getState()
        .paneActions.activateEditorTabInPane(windowPaneStore.getState().activePaneId, bFirst)
    })
    act(() => {
      openFile('w2', '/src/other.ts')
    })

    const entries = useJumpListStore.getState().entries
    expect(entries).toHaveLength(1)
    expect(entries[0].bufferId).toBe(bFirst)
    expect(entries[0].workspaceId).toBe('w2')
  })
})
