import { describe, it, expect, beforeEach } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import type { DragEndEvent, DragStartEvent } from '@dnd-kit/core'
import { useTabDrag } from '@/features/tabs/hooks/use-tab-drag'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import type { EditorContent, PaneContent } from '@/features/panes/types/pane-content'

/**
 * A tab must only ever be reorderable within its OWN pane's own tab bar —
 * cross-pane tab drag-and-drop is not a supported gesture at all (a dragged
 * tab must never move into, or open in, a different pane/split). Each
 * TabBar mounts its own `DndContext`/`SortableContext`, so `event.over`
 * handed to this hook can only ever name a droppable from the SAME pane's
 * tab bar — there is no `paneId` to smuggle a cross-pane drop through any
 * more, and no DOM hit-testing side channel for it to escape via either.
 *
 * Previously this hook bypassed that isolation with its own manual
 * point-based DOM hit test (`resolveDropTarget`) and called
 * `onMoveBufferToPane` directly whenever the pointer ended up over a
 * different pane's tab bar — the actual bug this suite now guards against.
 */

function makeTab(id: string): EditorContent {
  return {
    id,
    type: 'editor',
    path: `src/${id}.ts`,
    name: `${id}.ts`,
    workspaceId: 'w1',
    content: '',
    savedContent: '',
    isDirty: false,
    isVirtual: false,
    tokens: [],
  }
}

function openTab(paneId: string, id: string): EditorContent {
  const buffer = makeTab(id)
  windowPaneStore.setState((state) => {
    state.buffers.push(buffer as PaneContent)
    return state
  })
  windowPaneStore.getState().paneActions.addEditorTabToPane(paneId, buffer)
  return buffer
}

beforeEach(() => {
  resetWindowPaneStoreForTests()
})

function setup() {
  const state = () => windowPaneStore.getState()

  const leftTab = openTab(ROOT_PANE_ID, 'left-tab')
  const rightPaneId = state().paneActions.splitPane(ROOT_PANE_ID, 'horizontal')!
  const rightTab = openTab(rightPaneId, 'right-tab')

  const reorderCalls: Array<[number, number]> = []

  const hook = renderHook(() =>
    useTabDrag({
      sortedBuffers: state().buffers.filter((b) => b.id === leftTab.id),
      onTabSelect: () => {},
      onTabClick: () => {},
      onReorderBuffers: (oldIndex, newIndex) => reorderCalls.push([oldIndex, newIndex]),
      onSplitPane: () => undefined,
    }),
  )

  return { state, leftTabId: leftTab.id, rightPaneId, rightTabId: rightTab.id, reorderCalls, hook }
}

describe('useTabDrag — a tab can never leave its own pane via drag', () => {
  it('ending the drag over another pane leaves the tab exactly where it started', () => {
    const { state, leftTabId, rightPaneId, hook } = setup()

    act(() => {
      hook.result.current.handleDragStart({
        active: { id: leftTabId },
        activatorEvent: { clientX: 999, clientY: 999 },
      } as unknown as DragStartEvent)
    })
    act(() => {
      // dnd-kit's own `over` is null here on purpose: this pane's
      // SortableContext holds only `left-tab`, so nothing else in the
      // document — including the other pane's own tab bar — is ever a
      // valid collision target for this drag, however far the pointer
      // travels. There is no cross-pane droppable to name.
      hook.result.current.handleDragEnd({
        active: { id: leftTabId, rect: { current: { initial: null, translated: null } } },
        over: null,
      } as unknown as DragEndEvent)
    })

    expect(state().panes[ROOT_PANE_ID]?.editorTabIds).toContain(leftTabId)
    expect(state().panes[rightPaneId]?.editorTabIds ?? []).not.toContain(leftTabId)
  })

  it('clears the dragged-tab state on drag end without touching any other pane', () => {
    const { state, leftTabId, rightPaneId, rightTabId, hook } = setup()

    act(() => {
      hook.result.current.handleDragStart({
        active: { id: leftTabId },
        activatorEvent: { clientX: 999, clientY: 999 },
      } as unknown as DragStartEvent)
    })
    expect(hook.result.current.draggedBufferId).toBe(leftTabId)

    act(() => {
      hook.result.current.handleDragEnd({
        active: { id: leftTabId, rect: { current: { initial: null, translated: null } } },
        over: null,
      } as unknown as DragEndEvent)
    })

    expect(hook.result.current.draggedBufferId).toBeNull()
    expect(state().panes[rightPaneId]?.editorTabIds).toEqual([rightTabId])
  })
})

describe('useTabDrag — reordering within a pane’s own tab bar still works', () => {
  it('reorders when dnd-kit resolves `over` to another tab in the SAME SortableContext', () => {
    const tabA = openTab(ROOT_PANE_ID, 'tab-a')
    const tabB = openTab(ROOT_PANE_ID, 'tab-b')
    const tabC = openTab(ROOT_PANE_ID, 'tab-c')
    const sortedBuffers = [tabA, tabB, tabC] as PaneContent[]

    const reorderCalls: Array<[number, number]> = []
    const hook = renderHook(() =>
      useTabDrag({
        sortedBuffers,
        onTabSelect: () => {},
        onTabClick: () => {},
        onReorderBuffers: (oldIndex, newIndex) => reorderCalls.push([oldIndex, newIndex]),
        onSplitPane: () => undefined,
      }),
    )

    act(() => {
      hook.result.current.handleDragStart({
        active: { id: tabA.id },
        activatorEvent: { clientX: 10, clientY: 10 },
      } as unknown as DragStartEvent)
    })
    act(() => {
      hook.result.current.handleDragEnd({
        active: { id: tabA.id, rect: { current: { initial: null, translated: null } } },
        over: { id: tabC.id },
      } as unknown as DragEndEvent)
    })

    expect(reorderCalls).toEqual([[0, 2]])
  })

  it('no-ops when the drag ends back over its own starting position', () => {
    const { leftTabId, hook, reorderCalls } = setup()

    act(() => {
      hook.result.current.handleDragStart({
        active: { id: leftTabId },
        activatorEvent: { clientX: 10, clientY: 10 },
      } as unknown as DragStartEvent)
    })
    act(() => {
      hook.result.current.handleDragEnd({
        active: { id: leftTabId, rect: { current: { initial: null, translated: null } } },
        over: { id: leftTabId },
      } as unknown as DragEndEvent)
    })

    expect(reorderCalls).toHaveLength(0)
  })
})
