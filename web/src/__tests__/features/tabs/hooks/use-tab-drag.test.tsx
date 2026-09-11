import { describe, it, expect, afterEach, beforeEach, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import type { DragEndEvent, DragMoveEvent, DragStartEvent } from '@dnd-kit/core'
import { useTabDrag } from '@/features/tabs/hooks/use-tab-drag'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import type { EditorContent, PaneContent } from '@/features/panes/types/pane-content'
import { getInternalTabDragHover } from '@/features/tabs/utils/internal-tab-drag'

/**
 * Dropping a tab on another pane runs two store calls back to back:
 * `moveEditorTabToPane` and then `activateEditorTabInPane(dest, dragged.id)`.
 * The pane renders only what is in its own `editorTabIds`, so an
 * `activeEditorTabId` outside that list draws the empty-pane fallback WITH a
 * populated tab strip above it — tabs visible, none selected, nothing rendered.
 * These pin that the pair of calls always leaves the destination on a tab it
 * genuinely holds.
 *
 * Migrated in the final fix wave: this suite drove `createWorkspaceStore('w1')`
 * (panes moved to the window-level store in Task 26) and the pre-Task-1 action
 * names, so every case threw. Its original subject — a 'newTab' PLACEHOLDER
 * buffer being deduped on drop — is gone with the type itself (Task 1 removed
 * 'newTab' from PaneContent; a pane with zero editorTabIds renders the New Tab
 * stage for free), so the third case is deleted rather than rewritten; the
 * invariant the first two guard is unchanged and still live.
 */

const point = { clientX: 40, clientY: 12 }

function dropTargetElement(paneId: string): HTMLElement {
  const el = document.createElement('div')
  el.setAttribute('data-tab-bar-pane-id', paneId)
  document.body.appendChild(el)
  return el
}

/** jsdom has no layout engine and therefore no `elementsFromPoint` at all, so
 *  it has to be installed rather than spied on. The drop resolver only ever asks
 *  it "what is under the pointer", which is exactly what this answers. */
function stubElementsFromPoint(elements: HTMLElement[]) {
  ;(document as unknown as { elementsFromPoint: () => Element[] }).elementsFromPoint = () =>
    elements
}

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

afterEach(() => {
  document.body.innerHTML = ''
  delete (document as unknown as { elementsFromPoint?: unknown }).elementsFromPoint
  vi.restoreAllMocks()
})

function setup() {
  const state = () => windowPaneStore.getState()

  const leftTab = openTab(ROOT_PANE_ID, 'left-tab')
  const rightPaneId = state().paneActions.splitPane(ROOT_PANE_ID, 'horizontal')!
  const rightTab = openTab(rightPaneId, 'right-tab')

  stubElementsFromPoint([dropTargetElement(rightPaneId)])

  const hook = renderHook(() =>
    useTabDrag({
      paneId: ROOT_PANE_ID,
      sortedBuffers: state().buffers.filter((b) => b.id === leftTab.id),
      onTabSelect: () => {},
      onTabClick: () => {},
      onReorderBuffers: () => {},
      onMoveBufferToPane: (bufferId, fromPaneId, toPaneId) =>
        state().paneActions.moveEditorTabToPane(bufferId, fromPaneId, toPaneId),
      onActivatePaneBuffer: (paneId, bufferId) =>
        state().paneActions.activateEditorTabInPane(paneId, bufferId),
      onSplitPane: (targetPaneId, direction, bufferId, placement) =>
        state().paneActions.splitPane(targetPaneId, direction, bufferId, placement) ?? undefined,
    }),
  )

  const drop = () => {
    act(() => {
      hook.result.current.handleDragStart({
        active: { id: leftTab.id },
        activatorEvent: point,
      } as unknown as DragStartEvent)
    })
    act(() => {
      hook.result.current.handleDragEnd({
        active: { id: leftTab.id, rect: { current: { initial: null, translated: null } } },
        over: null,
      } as unknown as DragEndEvent)
    })
  }

  return { state, leftTabId: leftTab.id, rightPaneId, rightTabId: rightTab.id, drop }
}

describe('useTabDrag — dropping a tab on another pane', () => {
  it('leaves the destination pointing at a tab that still exists', () => {
    const { state, rightPaneId, drop } = setup()

    drop()

    const pane = state().panes[rightPaneId]
    expect(pane?.activeEditorTabId).not.toBeNull()
    expect(state().buffers.some((b) => b.id === pane!.activeEditorTabId)).toBe(true)
  })

  it('leaves the destination pointing at a tab the pane actually holds', () => {
    const { state, leftTabId, rightPaneId, rightTabId, drop } = setup()

    drop()

    const pane = state().panes[rightPaneId]
    // A pane renders only tabs in its own editorTabIds, so an activeEditorTabId
    // outside that list draws the empty-pane fallback with a tab strip showing.
    expect(pane?.editorTabIds).toContain(pane?.activeEditorTabId)
    expect(pane?.editorTabIds).toEqual([rightTabId, leftTabId])
    expect(pane?.activeEditorTabId).toBe(leftTabId)
  })

  it('takes the tab OUT of the source pane', () => {
    const { state, leftTabId, drop } = setup()

    drop()

    expect(state().panes[ROOT_PANE_ID]?.editorTabIds).not.toContain(leftTabId)
  })

  // DELETED (final fix wave): 'still drops the duplicate rather than stacking
  // two blank tabs'. It asserted that moving a 'newTab' PLACEHOLDER onto a pane
  // that already had one deleted the dragged buffer. The placeholder type no
  // longer exists (Task 1), so there is no duplicate to drop and no buffer a
  // move is allowed to delete — `moveEditorTabToPane` only ever moves.
})

/** A `[data-pane-id]` element sized/positioned so a point can be aimed at a
 *  specific normalized zone within it via `getPaneDropZoneFromRect`. */
function paneBodyElement(
  paneId: string,
  rect: { left: number; top: number; width: number; height: number },
): HTMLElement {
  const el = document.createElement('div')
  el.setAttribute('data-pane-id', paneId)
  Object.defineProperty(el, 'getBoundingClientRect', {
    value: () => ({ ...rect, right: rect.left + rect.width, bottom: rect.top + rect.height }),
  })
  document.body.appendChild(el)
  return el
}

describe('useTabDrag — hover zone published during a drag', () => {
  // Spec §7.3 (Law 3): "a pane group is a group of chats, never of tabs" — a
  // dragged tab can only ever land inside a pane's existing tab group, never
  // split it. Publishing the RAW edge zone here would light up
  // SplitDropOverlay's directional "this will create a new pane" quadrant —
  // the same affordance a chat drag gets — even though dropping a tab there
  // silently just moves it in. The published zone must always read 'center'
  // for a tab drag, whatever edge the pointer is actually over.
  it('normalizes an edge hover over another pane to "center" — tabs never publish a split zone', () => {
    const leftTab = openTab(ROOT_PANE_ID, 'left-tab')
    const rightPaneId = windowPaneStore.getState().paneActions.splitPane(
      ROOT_PANE_ID,
      'horizontal',
    )!

    // Point at x=5 within a 200-wide pane rect starting at x=0: nx = 0.025,
    // well inside the 0.25 threshold for the 'left' edge zone.
    stubElementsFromPoint([paneBodyElement(rightPaneId, { left: 0, top: 0, width: 200, height: 100 })])

    const hook = renderHook(() =>
      useTabDrag({
        paneId: ROOT_PANE_ID,
        sortedBuffers: [leftTab],
        onTabSelect: () => {},
        onTabClick: () => {},
        onReorderBuffers: () => {},
        onMoveBufferToPane: () => {},
        onActivatePaneBuffer: () => {},
        onSplitPane: () => undefined,
      }),
    )

    act(() => {
      hook.result.current.handleDragStart({
        active: { id: leftTab.id },
        activatorEvent: { clientX: 5, clientY: 50 },
      } as unknown as DragStartEvent)
    })
    act(() => {
      hook.result.current.handleDragMove({
        active: { id: leftTab.id, rect: { current: { initial: null, translated: null } } },
      } as unknown as DragMoveEvent)
    })

    expect(getInternalTabDragHover()).toEqual({ paneId: rightPaneId, zone: 'center' })
  })
})
