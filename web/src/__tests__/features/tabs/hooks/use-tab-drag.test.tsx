import { describe, it, expect, afterEach, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import type { DragEndEvent, DragStartEvent } from '@dnd-kit/core'
import { useTabDrag } from '@/features/tabs/hooks/use-tab-drag'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'

/**
 * Dropping a tab on another pane runs two store calls back to back:
 * `moveBufferToPane` and then `activatePaneBuffer(dest, dragged.id)`. When the
 * dragged tab is a New Tab and the destination already holds one, the move
 * DELETES the dragged buffer (a New Tab is a placeholder, not content) and
 * points the destination at its own — and the follow-up activation then
 * overwrites that with the id of the buffer that no longer exists. The pane
 * renders its `!activeBuffer` fallback while the strip shows a tab in the
 * inactive style with nothing selected.
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

afterEach(() => {
  document.body.innerHTML = ''
  delete (document as unknown as { elementsFromPoint?: unknown }).elementsFromPoint
  vi.restoreAllMocks()
})

function setup() {
  const store = createWorkspaceStore('w1')
  const state = () => store.getState()

  const leftNewTabId = state().bufferActions.openNewTab(ROOT_PANE_ID)!
  const rightPaneId = state().paneActions.splitPane(ROOT_PANE_ID, 'horizontal')!
  const rightNewTabId = state().bufferActions.openNewTab(rightPaneId)!

  stubElementsFromPoint([dropTargetElement(rightPaneId)])

  const hook = renderHook(() =>
    useTabDrag({
      paneId: ROOT_PANE_ID,
      sortedBuffers: state().buffers.filter((b) => b.id === leftNewTabId),
      onTabSelect: () => {},
      onTabClick: () => {},
      onReorderBuffers: () => {},
      onMoveBufferToPane: (bufferId, fromPaneId, toPaneId) =>
        state().paneActions.moveBufferToPane(bufferId, fromPaneId, toPaneId),
      onActivatePaneBuffer: (paneId, bufferId) =>
        state().paneActions.activatePaneBuffer(paneId, bufferId),
      onSplitPane: (targetPaneId, direction, bufferId, placement) =>
        state().paneActions.splitPane(targetPaneId, direction, bufferId, placement) ?? undefined,
    }),
  )

  const drop = () => {
    act(() => {
      hook.result.current.handleDragStart({
        active: { id: leftNewTabId },
        activatorEvent: point,
      } as unknown as DragStartEvent)
    })
    act(() => {
      hook.result.current.handleDragEnd({
        active: { id: leftNewTabId, rect: { current: { initial: null, translated: null } } },
        over: null,
      } as unknown as DragEndEvent)
    })
  }

  return { store, state, leftNewTabId, rightPaneId, rightNewTabId, drop }
}

describe('useTabDrag — dropping a New Tab on a pane that already has one', () => {
  it('leaves the destination pointing at a buffer that still exists', () => {
    const { state, rightPaneId, drop } = setup()

    drop()

    const pane = state().panes[rightPaneId]
    expect(pane?.activeBufferId).not.toBeNull()
    expect(state().buffers.some((b) => b.id === pane!.activeBufferId)).toBe(true)
  })

  it('leaves the destination pointing at a buffer the pane actually holds', () => {
    const { state, rightPaneId, rightNewTabId, drop } = setup()

    drop()

    const pane = state().panes[rightPaneId]
    // A pane renders only buffers in its own bufferIds, so an activeBufferId
    // outside that list draws the empty-pane fallback with a tab strip showing.
    expect(pane?.bufferIds).toContain(pane?.activeBufferId)
    expect(pane?.activeBufferId).toBe(rightNewTabId)
  })

  it('still drops the duplicate rather than stacking two blank tabs', () => {
    const { state, leftNewTabId, rightPaneId, drop } = setup()

    drop()

    expect(state().buffers.some((b) => b.id === leftNewTabId)).toBe(false)
    expect(state().panes[rightPaneId]?.bufferIds).toHaveLength(1)
  })
})
