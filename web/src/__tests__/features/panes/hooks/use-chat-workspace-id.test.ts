import { act, cleanup, renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))

import {
  useActivePaneWorkspaceId,
  usePaneEditorWorkspaceIds,
  useViewWorkspaceIds,
} from '@/features/panes/hooks/use-chat-workspace-id'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import type { EditorContent } from '@/features/panes/types/pane-content'

const editorTab = (id: string, wsId: string): EditorContent => ({
  id,
  type: 'editor',
  name: id,
  path: id,
  workspaceId: wsId,
  content: '',
  savedContent: '',
  isDirty: false,
  isVirtual: false,
  tokens: [],
})

const paneActions = () => windowPaneStore.getState().paneActions

afterEach(() => {
  cleanup()
  resetWindowPaneStoreForTests()
})

describe('useActivePaneWorkspaceId', () => {
  it('follows the focused pane’s recorded workspace, re-rendering only when it moves', () => {
    act(() => {
      paneActions().openChat('c1', { workspaceId: 'ws-a' })
      paneActions().dropChatOnPane('c2', windowPaneStore.getState().activePaneId, 'right', 'ws-a')
      paneActions().dropChatOnPane('c3', windowPaneStore.getState().activePaneId, 'right', 'ws-b')
    })
    let renders = 0
    const { result } = renderHook(() => {
      renders++
      return useActivePaneWorkspaceId()
    })
    expect(result.current).toBe('ws-b')
    const panes = Object.values(windowPaneStore.getState().panes)
    const paneOf = (chatId: string) => panes.find((p) => p.chatId === chatId)!.id

    act(() => paneActions().setActivePane(paneOf('c1')))
    expect(result.current).toBe('ws-a')
    const afterMove = renders
    // Another chat of the same workspace: the answer does not move.
    act(() => paneActions().setActivePane(paneOf('c2')))
    expect(renders).toBe(afterMove)
  })
})

/**
 * `WorkspaceHost`'s "in a view" retention input, read straight off the view
 * members' records — no workspace store is scanned.
 */
describe('useViewWorkspaceIds', () => {
  it('is empty with no views', () => {
    const { result } = renderHook(() => useViewWorkspaceIds())
    expect(result.current).toEqual([])
  })

  it('includes the workspace a chat was opened with', () => {
    const { result } = renderHook(() => useViewWorkspaceIds())
    act(() => paneActions().openChat('c1', { workspaceId: 'ws-a' }))
    expect(result.current).toEqual(['ws-a'])
  })

  it('includes the workspace of a chat adopted as a background record', () => {
    const { result } = renderHook(() => useViewWorkspaceIds())
    act(() => paneActions().adoptBackgroundChat('c1', 'p1', 'ws-a'))
    expect(result.current).toEqual(['ws-a'])
  })

  it('drops a workspace the instant its last member closes', () => {
    act(() => paneActions().openChat('c1', { workspaceId: 'ws-a' }))
    const { result } = renderHook(() => useViewWorkspaceIds())
    expect(result.current).toEqual(['ws-a'])
    act(() => paneActions().closePane(windowPaneStore.getState().activePaneId))
    expect(result.current).toEqual([])
  })

  it('unions members across workspaces', () => {
    act(() => {
      paneActions().openChat('c1', { workspaceId: 'ws-a' })
      paneActions().openChat('c2', { workspaceId: 'ws-b' })
    })
    const { result } = renderHook(() => useViewWorkspaceIds())
    expect(result.current).toEqual(['ws-a', 'ws-b'])
  })

  it('does not re-render on a pane write that moves no workspace (a stream frame)', () => {
    act(() => paneActions().openChat('c1', { workspaceId: 'ws-a' }))
    let renders = 0
    renderHook(() => {
      renders++
      return useViewWorkspaceIds()
    })
    const before = renders
    act(() => {
      for (let i = 0; i < 20; i++) {
        paneActions().setPaneRunner(windowPaneStore.getState().activePaneId, `r-${i}`)
      }
    })
    expect(renders).toBe(before)
  })
})

// Regression: an editor-only pane (chatId: null, real editorTabIds) names no
// chat at all, so it was invisible to WorkspaceHost's retention set entirely
// — planRetention (keep-alive-policy.ts) could evict a workspace still
// displaying an open file/terminal split the moment its chat (if any)
// dropped out of Recents, destroying the store (and EditorSurface's
// editorManager) out from under the still-visible pane. Live-reported as
// "Editor failed to load. Try closing and reopening this file."
describe('usePaneEditorWorkspaceIds', () => {
  it('names the workspace an editor-only pane (no chat) holds a file for', () => {
    act(() => {
      windowPaneStore.setState((state) => ({
        buffers: [...state.buffers, editorTab('tab-1', 'ws-a')],
      }))
      windowPaneStore.getState().paneActions.splitPane(ROOT_PANE_ID, 'horizontal', 'tab-1')
    })

    const { result } = renderHook(() => usePaneEditorWorkspaceIds())

    expect(result.current).toEqual(['ws-a'])
  })

  it('unions across every pane and drops a workspace once its tab closes', () => {
    let paneA = ''
    act(() => {
      windowPaneStore.setState((state) => ({
        buffers: [...state.buffers, editorTab('tab-a', 'ws-a'), editorTab('tab-b', 'ws-b')],
        panes: {
          ...state.panes,
          [ROOT_PANE_ID]: { ...state.panes[ROOT_PANE_ID]!, editorTabIds: ['tab-a', 'tab-b'] },
        },
      }))
      const { paneActions } = windowPaneStore.getState()
      paneA = paneActions.splitPane(ROOT_PANE_ID, 'horizontal', 'tab-a')!
      paneActions.splitPane(ROOT_PANE_ID, 'vertical', 'tab-b')
      paneActions.removeEditorTabFromPane(ROOT_PANE_ID, 'tab-a')
      paneActions.removeEditorTabFromPane(ROOT_PANE_ID, 'tab-b')
    })
    const { result, rerender } = renderHook(() => usePaneEditorWorkspaceIds())
    expect([...result.current].sort()).toEqual(['ws-a', 'ws-b'])

    act(() => {
      windowPaneStore.getState().paneActions.removeEditorTabFromPane(paneA, 'tab-a')
    })
    rerender()

    expect(result.current).toEqual(['ws-b'])
  })

  it('answers empty when no pane holds any editor tab', () => {
    const { result } = renderHook(() => usePaneEditorWorkspaceIds())
    expect(result.current).toEqual([])
  })
})
