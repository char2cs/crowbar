import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { createRef } from 'react'

vi.mock('@/lib/crowbar-bridge', () => ({
  isTauri: vi.fn(),
  browserPaneSync: vi.fn().mockResolvedValue(undefined),
  browserPaneClose: vi.fn().mockResolvedValue(undefined),
}))

import { isTauri, browserPaneSync, browserPaneClose } from '@/lib/crowbar-bridge'
import { useBrowserPaneAnchor } from '@/features/web-viewer/hooks/use-browser-pane-anchor'

// Minimal ResizeObserver mock
let resizeCallback: ResizeObserverCallback | null = null
class MockResizeObserver {
  constructor(cb: ResizeObserverCallback) { resizeCallback = cb }
  observe() {}
  disconnect() { resizeCallback = null }
}

beforeEach(() => {
  vi.stubGlobal('ResizeObserver', MockResizeObserver)
  vi.clearAllMocks()
  // Mock requestAnimationFrame to run synchronously
  vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => { cb(0); return 0 })
  vi.stubGlobal('cancelAnimationFrame', vi.fn())
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('useBrowserPaneAnchor — non-Tauri env', () => {
  it('returns isTauri=false and does not call browserPaneSync', () => {
    ;(isTauri as ReturnType<typeof vi.fn>).mockReturnValue(false)
    const ref = createRef<HTMLDivElement>()
    const { result } = renderHook(() =>
      useBrowserPaneAnchor({ bufferId: 'b1', isVisible: true, anchorRef: ref }),
    )
    expect(result.current.isTauri).toBe(false)
    expect(browserPaneSync).not.toHaveBeenCalled()
  })
})

describe('useBrowserPaneAnchor — Tauri env', () => {
  beforeEach(() => {
    ;(isTauri as ReturnType<typeof vi.fn>).mockReturnValue(true)
  })

  it('returns isTauri=true', () => {
    const ref = createRef<HTMLDivElement>()
    const { result } = renderHook(() =>
      useBrowserPaneAnchor({ bufferId: 'b1', isVisible: true, anchorRef: ref }),
    )
    expect(result.current.isTauri).toBe(true)
  })

  it('calls browserPaneSync on mount with visible=true', () => {
    const div = document.createElement('div')
    div.getBoundingClientRect = () =>
      ({ x: 10, y: 20, width: 400, height: 300 } as DOMRect)
    const ref = { current: div }
    renderHook(() =>
      useBrowserPaneAnchor({ bufferId: 'b1', isVisible: true, anchorRef: ref }),
    )
    expect(browserPaneSync).toHaveBeenCalledWith(
      'b1',
      { x: 10, y: 20, width: 400, height: 300 },
      true,
    )
  })

  it('does not call browserPaneSync when only isVisible changes (sync happens on next resize)', () => {
    const div = document.createElement('div')
    div.getBoundingClientRect = () =>
      ({ x: 0, y: 0, width: 100, height: 100 } as DOMRect)
    const ref = { current: div }
    const { rerender } = renderHook(
      ({ visible }: { visible: boolean }) =>
        useBrowserPaneAnchor({ bufferId: 'b1', isVisible: visible, anchorRef: ref }),
      { initialProps: { visible: true } },
    )
    vi.clearAllMocks()
    rerender({ visible: false })
    // No sync is triggered by the prop change alone — the ref is updated silently
    expect(browserPaneSync).not.toHaveBeenCalled()
  })

  it('calls browserPaneClose on unmount', () => {
    const ref = createRef<HTMLDivElement>()
    const { unmount } = renderHook(() =>
      useBrowserPaneAnchor({ bufferId: 'b1', isVisible: true, anchorRef: ref }),
    )
    unmount()
    expect(browserPaneClose).toHaveBeenCalledWith('b1')
  })

  it('uses the current isVisible value when ResizeObserver fires after visibility changes', () => {
    const div = document.createElement('div')
    div.getBoundingClientRect = () => ({ x: 0, y: 0, width: 100, height: 100 } as DOMRect)
    const ref = { current: div }
    const { rerender } = renderHook(
      ({ visible }: { visible: boolean }) =>
        useBrowserPaneAnchor({ bufferId: 'b1', isVisible: visible, anchorRef: ref }),
      { initialProps: { visible: true } },
    )
    // Flip visibility
    rerender({ visible: false })
    vi.clearAllMocks()
    // Simulate ResizeObserver firing AFTER the flip
    if (resizeCallback) resizeCallback([], null as unknown as ResizeObserver)
    expect(browserPaneSync).toHaveBeenCalledWith('b1', expect.any(Object), false)
  })
})
