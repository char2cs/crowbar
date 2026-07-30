/**
 * Tests for PaneSash unmount-mid-drag cleanup (R21).
 *
 * Verifies:
 *   1. Starting a drag then unmounting removes window listeners and clears
 *      the data-pane-resizing attribute.
 *   2. Unmounting without an active drag does NOT touch data-pane-resizing.
 *
 * Note: jsdom does not implement the PointerEvent constructor. We dispatch
 * `MouseEvent` instances with type `"pointerdown"` / `"pointermove"` —
 * React's synthetic event system and the native window listeners handle
 * both identically because they duck-type the event by name.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { MockInstance } from 'vitest'
import { render, cleanup } from '@testing-library/react'
import React from 'react'
import { PaneSash } from '@/features/panes/components/pane-sash'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeDivRef(width = 400, height = 300) {
  const el = document.createElement('div')
  Object.defineProperty(el, 'getBoundingClientRect', {
    value: () => ({ left: 0, top: 0, width, height, right: width, bottom: height }),
  })
  Object.defineProperty(el, 'offsetWidth', { value: 4 })
  Object.defineProperty(el, 'offsetHeight', { value: 4 })
  document.body.appendChild(el)
  const ref = { current: el } as React.MutableRefObject<HTMLDivElement | null>
  return ref
}

function renderSash(overrides: Partial<Parameters<typeof PaneSash>[0]> = {}) {
  const containerRef = makeDivRef(400, 300)
  const firstPaneRef = makeDivRef()
  const secondPaneRef = makeDivRef()
  const onResizeCommit = vi.fn()

  const props = {
    direction: 'horizontal' as const,
    sizes: [50, 50] as [number, number],
    containerRef,
    firstPaneRef,
    secondPaneRef,
    onResizeCommit,
    ...overrides,
  }

  const result = render(<PaneSash {...props} />)
  return { ...result, onResizeCommit, containerRef, firstPaneRef, secondPaneRef }
}

/**
 * Fire a pointerdown on an element in a way that React's synthetic event
 * system picks it up AND correctly sets e.button = 0.
 *
 * jsdom does not implement the PointerEvent constructor, so we use a plain
 * MouseEvent dispatched with type "pointerdown" — React normalises both.
 */
function fireDragStart(element: HTMLElement) {
  const evt = new MouseEvent('pointerdown', {
    button: 0,
    clientX: 200,
    clientY: 0,
    bubbles: true,
    cancelable: true,
  })
  element.dispatchEvent(evt)
}

/** Fire a pointermove on window with real movement, as the drag listener expects. */
function fireDragMove(clientX = 210) {
  const evt = new MouseEvent('pointermove', { clientX, clientY: 0, bubbles: true })
  window.dispatchEvent(evt)
}

/** Fire a pointerup on window, as the drag listener expects. */
function fireDragEnd(clientX = 200) {
  const evt = new MouseEvent('pointerup', { clientX, clientY: 0, bubbles: true })
  window.dispatchEvent(evt)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('PaneSash unmount-mid-drag cleanup', () => {
  // Spelled out rather than `ReturnType<typeof vi.spyOn>`: vi.spyOn is
  // overloaded, so bare ReturnType resolves the generic to its constraint and
  // yields Mock<Procedure> — whose mock.calls is any[], which makes the
  // `([type]) => …` destructures below implicit-any errors under Vitest 4.
  let removeEventListenerSpy: MockInstance<typeof window.removeEventListener>

  beforeEach(() => {
    // Ensure data-pane-resizing is not set from a previous test.
    document.documentElement.removeAttribute('data-pane-resizing')
    removeEventListenerSpy = vi.spyOn(window, 'removeEventListener')
  })

  afterEach(() => {
    removeEventListenerSpy.mockRestore()
    document.documentElement.removeAttribute('data-pane-resizing')
    cleanup()
    // Remove any test DOM nodes appended to body.
    document.body.innerHTML = ''
  })

  it('removes window listeners and clears data-pane-resizing when unmounted mid-drag', () => {
    const { container, unmount } = renderSash()
    const sash = container.querySelector('[data-slot="resizable-handle"]') as HTMLElement

    // Start a drag and actually move — the resizing attribute is only set
    // lazily, on the first real pointermove (see click-only test below).
    fireDragStart(sash)
    fireDragMove()

    // The resizing attribute should now be set.
    expect(document.documentElement.hasAttribute('data-pane-resizing')).toBe(true)

    // Unmount the component while drag is in progress.
    unmount()

    // The attribute must be cleared.
    expect(document.documentElement.hasAttribute('data-pane-resizing')).toBe(false)

    // Window listeners must have been removed (pointermove + pointerup + pointercancel).
    const removedEventTypes = removeEventListenerSpy.mock.calls.map(([type]) => type as string)
    expect(removedEventTypes).toContain('pointermove')
    expect(removedEventTypes).toContain('pointerup')
    expect(removedEventTypes).toContain('pointercancel')
  })

  it('does not fire pointermove handler after unmount mid-drag', () => {
    const { container, firstPaneRef, unmount } = renderSash()
    const sash = container.querySelector('[data-slot="resizable-handle"]') as HTMLElement

    fireDragStart(sash)
    unmount()

    // Record flex-basis state before firing a native pointermove.
    const flexBefore = firstPaneRef.current?.style.flexBasis ?? ''

    // Dispatch a native pointermove — it should be a no-op because the listener
    // was removed during unmount cleanup.
    const moveEvt = new MouseEvent('pointermove', {
      clientX: 300,
      clientY: 0,
      bubbles: true,
    })
    window.dispatchEvent(moveEvt)

    // flex-basis must be unchanged (listener was removed).
    expect(firstPaneRef.current?.style.flexBasis ?? '').toBe(flexBefore)
  })

  it('does NOT touch data-pane-resizing when unmounted with no active drag', () => {
    // Set the attribute manually to a sentinel value, simulating another
    // active sash that is legitimately dragging.
    document.documentElement.setAttribute('data-pane-resizing', 'other')

    const { unmount } = renderSash()

    // Unmount without ever starting a drag.
    unmount()

    // The attribute must remain untouched.
    expect(document.documentElement.getAttribute('data-pane-resizing')).toBe('other')

    // removeEventListener should NOT have been called for drag-related event types
    // as a result of this idle unmount.
    const relevantCalls = removeEventListenerSpy.mock.calls.filter(([type]) =>
      (['pointermove', 'pointerup', 'pointercancel'] as string[]).includes(type as string),
    )
    expect(relevantCalls).toHaveLength(0)
  })
})

describe('PaneSash click-only interaction (no drag)', () => {
  beforeEach(() => {
    document.documentElement.removeAttribute('data-pane-resizing')
  })

  afterEach(() => {
    document.documentElement.removeAttribute('data-pane-resizing')
    cleanup()
    document.body.innerHTML = ''
  })

  it('does not set data-pane-resizing on a bare click (pointerdown -> pointerup, no move)', () => {
    const { container } = renderSash()
    const sash = container.querySelector('[data-slot="resizable-handle"]') as HTMLElement

    fireDragStart(sash)
    fireDragEnd()

    expect(document.documentElement.hasAttribute('data-pane-resizing')).toBe(false)
  })

  it('does not commit sizes or dispatch pane-resize-end on a bare click', () => {
    const { container, onResizeCommit } = renderSash()
    const sash = container.querySelector('[data-slot="resizable-handle"]') as HTMLElement
    const resizeEndSpy = vi.fn()
    window.addEventListener('pane-resize-end', resizeEndSpy)

    fireDragStart(sash)
    fireDragEnd()

    expect(onResizeCommit).not.toHaveBeenCalled()
    expect(resizeEndSpy).not.toHaveBeenCalled()
    window.removeEventListener('pane-resize-end', resizeEndSpy)
  })

  it('still enters and exits the resizing state normally when the pointer actually moves', () => {
    const { container, onResizeCommit } = renderSash()
    const sash = container.querySelector('[data-slot="resizable-handle"]') as HTMLElement

    fireDragStart(sash)
    fireDragMove(210)
    expect(document.documentElement.hasAttribute('data-pane-resizing')).toBe(true)

    fireDragEnd(210)
    expect(document.documentElement.hasAttribute('data-pane-resizing')).toBe(false)
    expect(onResizeCommit).toHaveBeenCalledTimes(1)
  })
})
