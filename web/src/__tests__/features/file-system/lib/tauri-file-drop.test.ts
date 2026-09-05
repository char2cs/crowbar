import type { RefObject } from 'react'
import { describe, expect, it, beforeEach, vi } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { pointInRect, useTauriFileDrop } from '@/features/file-system/lib/tauri-file-drop'

function rect(left: number, top: number, right: number, bottom: number): DOMRect {
  return {
    left,
    top,
    right,
    bottom,
    width: right - left,
    height: bottom - top,
    x: left,
    y: top,
    toJSON: () => ({}),
  }
}

describe('pointInRect', () => {
  it('is true for a point strictly inside', () => {
    expect(pointInRect({ x: 50, y: 50 }, rect(0, 0, 100, 100))).toBe(true)
  })

  it('is true on the boundary (inclusive)', () => {
    expect(pointInRect({ x: 100, y: 100 }, rect(0, 0, 100, 100))).toBe(true)
    expect(pointInRect({ x: 0, y: 0 }, rect(0, 0, 100, 100))).toBe(true)
  })

  it('is false outside the rect', () => {
    expect(pointInRect({ x: 101, y: 50 }, rect(0, 0, 100, 100))).toBe(false)
    expect(pointInRect({ x: 50, y: -1 }, rect(0, 0, 100, 100))).toBe(false)
  })
})

// The hook's own effect body — dynamic import, the onDragDropEvent
// subscription, the payload-type filter, the null-ref guard, the rect
// filter and the disposed-flag unmount race — is exercised here directly,
// with @tauri-apps/api/webview and isTauri mocked at the module boundary
// (never the hook itself).
type DragDropCallback = (event: { payload: Record<string, unknown> }) => void

const { onDragDropEvent, stop, isTauriMock } = vi.hoisted(() => ({
  onDragDropEvent: vi.fn(),
  stop: vi.fn(),
  isTauriMock: vi.fn(() => true),
}))

vi.mock('@tauri-apps/api/webview', () => ({
  getCurrentWebview: () => ({ onDragDropEvent }),
}))

vi.mock('@/lib/crowbar-bridge', () => ({ isTauri: isTauriMock }))

function containerRefWithRect(r: DOMRect): RefObject<HTMLElement | null> {
  const el = document.createElement('div')
  el.getBoundingClientRect = () => r
  return { current: el }
}

describe('useTauriFileDrop', () => {
  beforeEach(() => {
    onDragDropEvent.mockReset()
    stop.mockReset()
    isTauriMock.mockReset()
    isTauriMock.mockReturnValue(true)
  })

  it('calls onDrop with the paths for a drop inside the container rect', async () => {
    let capturedCallback: DragDropCallback | undefined
    onDragDropEvent.mockImplementation(async (cb: DragDropCallback) => {
      capturedCallback = cb
      return stop
    })
    const containerRef = containerRefWithRect(rect(0, 0, 100, 100))
    const onDrop = vi.fn()

    renderHook(() => useTauriFileDrop(containerRef, onDrop))

    await waitFor(() => expect(capturedCallback).toBeDefined())
    capturedCallback!({ payload: { type: 'drop', position: { x: 50, y: 50 }, paths: ['/a/b.png'] } })

    expect(onDrop).toHaveBeenCalledWith(['/a/b.png'])
  })

  it('does not call onDrop for a drop outside the container rect', async () => {
    let capturedCallback: DragDropCallback | undefined
    onDragDropEvent.mockImplementation(async (cb: DragDropCallback) => {
      capturedCallback = cb
      return stop
    })
    const containerRef = containerRefWithRect(rect(0, 0, 100, 100))
    const onDrop = vi.fn()

    renderHook(() => useTauriFileDrop(containerRef, onDrop))

    await waitFor(() => expect(capturedCallback).toBeDefined())
    capturedCallback!({
      payload: { type: 'drop', position: { x: 500, y: 500 }, paths: ['/a/b.png'] },
    })

    expect(onDrop).not.toHaveBeenCalled()
  })

  it('ignores a non-drop payload type (e.g. over/cancel)', async () => {
    let capturedCallback: DragDropCallback | undefined
    onDragDropEvent.mockImplementation(async (cb: DragDropCallback) => {
      capturedCallback = cb
      return stop
    })
    const containerRef = containerRefWithRect(rect(0, 0, 100, 100))
    const onDrop = vi.fn()

    renderHook(() => useTauriFileDrop(containerRef, onDrop))

    await waitFor(() => expect(capturedCallback).toBeDefined())
    capturedCallback!({ payload: { type: 'over', position: { x: 50, y: 50 } } })
    capturedCallback!({ payload: { type: 'cancel' } })

    expect(onDrop).not.toHaveBeenCalled()
  })

  it('does not call onDrop when the container ref has gone null', async () => {
    let capturedCallback: DragDropCallback | undefined
    onDragDropEvent.mockImplementation(async (cb: DragDropCallback) => {
      capturedCallback = cb
      return stop
    })
    const containerRef: RefObject<HTMLElement | null> = { current: null }
    const onDrop = vi.fn()

    renderHook(() => useTauriFileDrop(containerRef, onDrop))

    await waitFor(() => expect(capturedCallback).toBeDefined())
    capturedCallback!({ payload: { type: 'drop', position: { x: 50, y: 50 }, paths: ['/a/b.png'] } })

    expect(onDrop).not.toHaveBeenCalled()
  })

  it('never subscribes when not running under Tauri', async () => {
    isTauriMock.mockReturnValue(false)
    const containerRef = containerRefWithRect(rect(0, 0, 100, 100))

    renderHook(() => useTauriFileDrop(containerRef, vi.fn()))

    // Give any stray microtask a chance to run before asserting the negative.
    await Promise.resolve()
    expect(onDragDropEvent).not.toHaveBeenCalled()
  })

  it('calls the returned stop() on unmount even when it races the async subscribe', async () => {
    let resolveSubscribe: ((stopFn: () => void) => void) | undefined
    onDragDropEvent.mockImplementation(
      () =>
        new Promise<() => void>((resolve) => {
          resolveSubscribe = resolve
        }),
    )
    const containerRef = containerRefWithRect(rect(0, 0, 100, 100))

    const { unmount } = renderHook(() => useTauriFileDrop(containerRef, vi.fn()))

    // Unmount BEFORE the getCurrentWebview().onDragDropEvent(...) promise
    // resolves — the disposed-flag race the effect's cleanup guards against.
    await waitFor(() => expect(onDragDropEvent).toHaveBeenCalled())
    unmount()
    expect(stop).not.toHaveBeenCalled()

    resolveSubscribe!(stop)
    await waitFor(() => expect(stop).toHaveBeenCalledTimes(1))
  })
})
