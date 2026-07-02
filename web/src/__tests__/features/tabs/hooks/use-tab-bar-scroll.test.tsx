import React from 'react'
import { render } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { useTabBarScroll } from '@/features/tabs/hooks/use-tab-bar-scroll'

// jsdom has no ResizeObserver; the hook only needs construct/observe/disconnect.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
vi.stubGlobal('ResizeObserver', ResizeObserverStub)

// Renders the hook against a tab-bar div whose getBoundingClientRect is
// stubbed to `rect`, and reports the edge flags.
function renderWithRect(rect: { left: number; right: number; top: number }) {
  const result: { isAtLeftEdge?: boolean; isAtRightEdge?: boolean; isAtTopEdge?: boolean } = {}

  function Probe() {
    const { tabBarRef, isAtLeftEdge, isAtRightEdge, isAtTopEdge } = useTabBarScroll({
      sidebarPosition: 'left',
      draggedBufferId: null,
    })
    result.isAtLeftEdge = isAtLeftEdge
    result.isAtRightEdge = isAtRightEdge
    result.isAtTopEdge = isAtTopEdge
    return (
      <div
        ref={(el) => {
          if (el) {
            el.getBoundingClientRect = vi.fn(
              () => ({ ...rect, bottom: rect.top + 44, width: 400, height: 44, x: rect.left, y: rect.top, toJSON: () => ({}) }) as DOMRect,
            )
          }
          tabBarRef.current = el
        }}
      />
    )
  }

  render(<Probe />)
  return result
}

describe('useTabBarScroll edge detection', () => {
  it('flags a tab bar at the window top-left (traffic-light zone)', () => {
    const r = renderWithRect({ left: 0, right: 400, top: 0 })
    expect(r.isAtLeftEdge).toBe(true)
    expect(r.isAtTopEdge).toBe(true)
  })

  it('does NOT flag the lower pane of a vertical split as top edge', () => {
    // Same left edge as the top pane, but hundreds of px down — it must not
    // reserve traffic-light space.
    const r = renderWithRect({ left: 0, right: 400, top: 420 })
    expect(r.isAtLeftEdge).toBe(true)
    expect(r.isAtTopEdge).toBe(false)
  })

  it('does not flag left edge for a right-column pane', () => {
    const r = renderWithRect({ left: 500, right: 900, top: 0 })
    expect(r.isAtLeftEdge).toBe(false)
    expect(r.isAtTopEdge).toBe(true)
  })
})
