import { act, render } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { usePaneTopRowEdges } from '@/features/tabs/hooks/use-pane-top-row-edges'

class FakeResizeObserver {
  static instances: FakeResizeObserver[] = []
  constructor(private callback: ResizeObserverCallback) {
    FakeResizeObserver.instances.push(this)
  }
  observe() {}
  unobserve() {}
  disconnect() {}
  trigger() {
    this.callback([], this as unknown as ResizeObserver)
  }
}
vi.stubGlobal('ResizeObserver', FakeResizeObserver)

const BOX = { left: 300, right: 700, top: 100, width: 400, height: 44 }
const NO_BOX = { left: 0, right: 0, top: 0, width: 0, height: 0 }

function mountRow(initial: typeof BOX) {
  FakeResizeObserver.instances = []
  const edges: { left?: boolean; top?: boolean } = {}
  let rect = initial
  function Probe() {
    const { rowRef, isAtLeftEdge, isAtTopEdge } = usePaneTopRowEdges()
    edges.left = isAtLeftEdge
    edges.top = isAtTopEdge
    return (
      <div
        ref={(el) => {
          if (el) el.getBoundingClientRect = () => rect as DOMRect
          rowRef.current = el
        }}
      />
    )
  }
  render(<Probe />)
  return {
    edges,
    setRect(next: typeof BOX) {
      rect = next
      act(() => FakeResizeObserver.instances[0]!.trigger())
    },
  }
}

describe('usePaneTopRowEdges', () => {
  it('keeps its last answer while the row has no box (a parked view)', () => {
    const row = mountRow(BOX)
    expect(row.edges).toEqual({ left: false, top: false })

    row.setRect(NO_BOX)

    expect(row.edges).toEqual({ left: false, top: false })
  })

  it('measures again once the row has a box back', () => {
    const row = mountRow(BOX)
    row.setRect(NO_BOX)

    row.setRect({ ...BOX, left: 4, top: 4 })

    expect(row.edges).toEqual({ left: true, top: true })
  })
})
