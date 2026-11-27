import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { MockInstance } from 'vitest'
import { render, cleanup } from '@testing-library/react'
import React from 'react'
import { PaneSash } from '@/features/panes/components/pane-sash'

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

function fireDragMove(clientX = 210) {
  const evt = new MouseEvent('pointermove', { clientX, clientY: 0, bubbles: true })
  window.dispatchEvent(evt)
}

function fireDragEnd(clientX = 200) {
  const evt = new MouseEvent('pointerup', { clientX, clientY: 0, bubbles: true })
  window.dispatchEvent(evt)
}

describe('PaneSash className passthrough', () => {
  afterEach(() => {
    cleanup()
    document.body.innerHTML = ''
  })

  it('merges a caller-supplied className onto the sash element', () => {
    const { container } = renderSash({ className: 'bg-chrome-bg' })
    const sash = container.querySelector('[data-slot="resizable-handle"]')
    expect(sash).toHaveClass('bg-chrome-bg')
  })
})

describe('PaneSash unmount-mid-drag cleanup', () => {
  let removeEventListenerSpy: MockInstance<typeof window.removeEventListener>

  beforeEach(() => {
    document.documentElement.removeAttribute('data-pane-resizing')
    removeEventListenerSpy = vi.spyOn(window, 'removeEventListener')
  })

  afterEach(() => {
    removeEventListenerSpy.mockRestore()
    document.documentElement.removeAttribute('data-pane-resizing')
    cleanup()
    document.body.innerHTML = ''
  })

  it('removes window listeners and clears data-pane-resizing when unmounted mid-drag', () => {
    const { container, unmount } = renderSash()
    const sash = container.querySelector('[data-slot="resizable-handle"]') as HTMLElement

    fireDragStart(sash)
    fireDragMove()

    expect(document.documentElement.hasAttribute('data-pane-resizing')).toBe(true)

    unmount()

    expect(document.documentElement.hasAttribute('data-pane-resizing')).toBe(false)

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

    const flexBefore = firstPaneRef.current?.style.flexBasis ?? ''

    const moveEvt = new MouseEvent('pointermove', {
      clientX: 300,
      clientY: 0,
      bubbles: true,
    })
    window.dispatchEvent(moveEvt)

    expect(firstPaneRef.current?.style.flexBasis ?? '').toBe(flexBefore)
  })

  it('does NOT touch data-pane-resizing when unmounted with no active drag', () => {
    document.documentElement.setAttribute('data-pane-resizing', 'other')

    const { unmount } = renderSash()

    unmount()

    expect(document.documentElement.getAttribute('data-pane-resizing')).toBe('other')

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

describe('PaneSash leaves the panes on their React-rendered flex-basis', () => {
  afterEach(() => {
    document.documentElement.removeAttribute('data-pane-resizing')
    cleanup()
    document.body.innerHTML = ''
  })

  function renderWithRenderedBasis() {
    const view = renderSash()
    view.firstPaneRef.current!.style.flexBasis = '50%'
    view.secondPaneRef.current!.style.flexBasis = '50%'
    const sash = view.container.querySelector('[data-slot="resizable-handle"]') as HTMLElement
    return { ...view, sash }
  }

  it('keeps the rendered percentages when the drag ends where it started', () => {
    const { sash, firstPaneRef, secondPaneRef, onResizeCommit } = renderWithRenderedBasis()

    fireDragStart(sash)
    fireDragMove(250)
    fireDragMove(198)
    fireDragEnd(198)

    expect(onResizeCommit).toHaveBeenCalledTimes(1)
    expect(firstPaneRef.current!.style.flexBasis).toBe('50%')
    expect(secondPaneRef.current!.style.flexBasis).toBe('50%')
  })

  it('restores the rendered percentages on pointercancel', () => {
    const { sash, firstPaneRef, secondPaneRef } = renderWithRenderedBasis()

    fireDragStart(sash)
    fireDragMove(250)
    window.dispatchEvent(new MouseEvent('pointercancel', { bubbles: true }))

    expect(firstPaneRef.current!.style.flexBasis).toBe('50%')
    expect(secondPaneRef.current!.style.flexBasis).toBe('50%')
  })

  it('restores the rendered percentages when unmounted mid-drag', () => {
    const { sash, firstPaneRef, secondPaneRef, unmount } = renderWithRenderedBasis()

    fireDragStart(sash)
    fireDragMove(250)
    unmount()

    expect(firstPaneRef.current!.style.flexBasis).toBe('50%')
    expect(secondPaneRef.current!.style.flexBasis).toBe('50%')
  })

  it('lets a changed commit be overwritten by the new rendered percentages', () => {
    const { sash, firstPaneRef, secondPaneRef, onResizeCommit } = renderWithRenderedBasis()
    onResizeCommit.mockImplementation(([a, b]: [number, number]) => {
      firstPaneRef.current!.style.flexBasis = `${a}%`
      secondPaneRef.current!.style.flexBasis = `${b}%`
    })

    fireDragStart(sash)
    fireDragMove(300)
    fireDragEnd(300)

    const [a, b] = onResizeCommit.mock.calls[0][0] as [number, number]
    expect(a).toBeGreaterThan(50)
    expect(firstPaneRef.current!.style.flexBasis).toBe(`${a}%`)
    expect(secondPaneRef.current!.style.flexBasis).toBe(`${b}%`)
  })
})
