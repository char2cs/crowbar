import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { observeRect } from '@/features/browser/lib/observe-rect'

class FakeResizeObserver {
  static instances: FakeResizeObserver[] = []
  disconnected = false
  constructor(public callback: ResizeObserverCallback) {
    FakeResizeObserver.instances.push(this)
  }
  observe() {}
  unobserve() {}
  disconnect() {
    this.disconnected = true
  }
}

class FakeIntersectionObserver {
  static instances: FakeIntersectionObserver[] = []
  disconnected = false
  constructor(
    public callback: IntersectionObserverCallback,
    public options: IntersectionObserverInit,
  ) {
    FakeIntersectionObserver.instances.push(this)
  }
  observe() {}
  disconnect() {
    this.disconnected = true
  }
  fire(ratio: number) {
    this.callback([{ intersectionRatio: ratio } as IntersectionObserverEntry], this as never)
  }
  static get live() {
    return FakeIntersectionObserver.instances.filter((o) => !o.disconnected)
  }
}

vi.stubGlobal('ResizeObserver', FakeResizeObserver)
vi.stubGlobal('IntersectionObserver', FakeIntersectionObserver)

let rect = { left: 100, top: 50, width: 400, height: 300 }
const node = document.createElement('div')
node.getBoundingClientRect = () => ({ ...rect, right: rect.left + rect.width }) as DOMRect

const disposers: Array<() => void> = []
const watch = (onChange: () => void) => {
  const dispose = observeRect(node, onChange)
  disposers.push(dispose)
  return dispose
}

afterEach(() => disposers.splice(0).forEach((d) => d()))

beforeEach(() => {
  FakeResizeObserver.instances = []
  FakeIntersectionObserver.instances = []
  rect = { left: 100, top: 50, width: 400, height: 300 }
})

describe('observeRect', () => {
  it('reports a resize of the node', () => {
    const onChange = vi.fn()
    watch(onChange)
    FakeResizeObserver.instances[0].callback([], {} as ResizeObserver)
    expect(onChange).toHaveBeenCalledTimes(1)
  })

  it('reports a move that does not resize, then watches the new position', () => {
    const onChange = vi.fn()
    watch(onChange)
    const first = FakeIntersectionObserver.live[0]
    first.fire(1) // the initial delivery is not a change
    expect(onChange).not.toHaveBeenCalled()

    rect = { ...rect, left: 260 }
    first.fire(0.6)

    expect(onChange).toHaveBeenCalledTimes(1)
    expect(first.disconnected).toBe(true)
    expect(FakeIntersectionObserver.live).toHaveLength(1)
  })

  it('reports a window resize', () => {
    const onChange = vi.fn()
    watch(onChange)
    window.dispatchEvent(new Event('resize'))
    expect(onChange).toHaveBeenCalledTimes(1)
  })

  it('stops everything on dispose', () => {
    const onChange = vi.fn()
    const dispose = watch(onChange)
    dispose()
    window.dispatchEvent(new Event('resize'))
    expect(onChange).not.toHaveBeenCalled()
    expect(FakeResizeObserver.instances[0].disconnected).toBe(true)
    expect(FakeIntersectionObserver.live).toHaveLength(0)
  })
})
