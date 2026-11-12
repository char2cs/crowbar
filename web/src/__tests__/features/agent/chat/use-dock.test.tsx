import { act, renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useDock } from '@/features/agent/chat/use-dock'

vi.mock('@/lib/layout-size', () => ({ layoutHeight: (el: HTMLElement) => el.offsetHeight }))

class Observer {
  observe() {}
  disconnect() {}
}

function dockAt(top: number, height: number): HTMLDivElement {
  const node = document.createElement('div')
  Object.defineProperty(node, 'offsetHeight', { value: height })
  node.getBoundingClientRect = () => ({ top }) as DOMRect
  return node
}

afterEach(() => vi.unstubAllGlobals())

describe('useDock', () => {
  it('reports where the dock’s top edge sits above the viewport, and follows the viewport', () => {
    vi.stubGlobal('ResizeObserver', Observer)
    const node = dockAt(700, 120)
    Object.defineProperty(window, 'innerHeight', { value: 850, configurable: true })
    const { result } = renderHook(() => useDock())
    act(() => result.current.dockRef(node))
    // 34 of the viewport below the dock is the safe area: not part of the dock's own height.
    expect(result.current.dockHeight).toBe(120)
    expect(result.current.dockTop).toBe(150)

    // The keyboard shrinks the viewport and the page's layout moves the dock up with it.
    Object.defineProperty(window, 'innerHeight', { value: 500, configurable: true })
    node.getBoundingClientRect = () => ({ top: 350 }) as DOMRect
    act(() => {
      window.dispatchEvent(new Event('resize'))
    })
    expect(result.current.dockTop).toBe(150)
    node.getBoundingClientRect = () => ({ top: 380 }) as DOMRect
    act(() => {
      window.dispatchEvent(new Event('resize'))
    })
    expect(result.current.dockTop).toBe(120)
  })
})
