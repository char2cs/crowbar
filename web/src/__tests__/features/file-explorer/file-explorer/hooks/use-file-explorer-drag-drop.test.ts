import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { MockInstance } from 'vitest'
import { useFileExplorerDragDrop } from '@/features/file-explorer/file-explorer/hooks/use-file-explorer-drag-drop'
import type { FileEntry } from '@/features/file-system/types/app'

// moveFile is the only async side effect on drop; stub it so a "drop on a
// directory" never hits the real platform controller.
vi.mock('@/features/file-system/controllers/platform', () => ({
  moveFile: vi.fn().mockResolvedValue(undefined),
}))

const makeFile = (name: string, path: string, isDir = false): FileEntry => ({
  name,
  path,
  isDir,
})

const mouseEvent = (type: string, x = 50, y = 50) =>
  new MouseEvent(type, { clientX: x, clientY: y, bubbles: true })

describe('useFileExplorerDragDrop — stable listener subscription (H13)', () => {
  // Spelled out rather than `ReturnType<typeof vi.spyOn>`: vi.spyOn is
  // overloaded, so bare ReturnType resolves the generic to its constraint and
  // yields Mock<Procedure> — whose mock.calls is any[], which makes the
  // `([type]) => …` destructures below implicit-any errors under Vitest 4.
  let addSpy: MockInstance<typeof document.addEventListener>
  let removeSpy: MockInstance<typeof document.removeEventListener>

  beforeEach(() => {
    addSpy = vi.spyOn(document, 'addEventListener')
    removeSpy = vi.spyOn(document, 'removeEventListener')
    // jsdom does not implement elementFromPoint; the handlers call it on every
    // move/up. Returning null = "empty space" (no drop target).
    ;(document as unknown as { elementFromPoint: () => Element | null }).elementFromPoint = () =>
      null
  })

  afterEach(() => {
    addSpy.mockRestore()
    removeSpy.mockRestore()
    vi.clearAllMocks()
    delete (document as unknown as { elementFromPoint?: unknown }).elementFromPoint
  })

  // Count addEventListener calls for the drag listeners only (ignore any other
  // listeners jsdom/React might attach).
  const countDragAdds = () =>
    addSpy.mock.calls.filter(
      ([type]) => type === 'mousemove' || type === 'mouseup' || type === 'mouseleave',
    ).length

  it('attaches each document listener exactly once per drag, not once per mousemove', () => {
    const { result } = renderHook(() => useFileExplorerDragDrop(undefined))

    // Start a drag.
    act(() => {
      const e = {
        preventDefault: vi.fn(),
        stopPropagation: vi.fn(),
        clientX: 10,
        clientY: 10,
      } as unknown as React.MouseEvent
      result.current.startDrag(e, makeFile('a.txt', '/repo/a.txt'))
    })

    expect(result.current.dragState.isDragging).toBe(true)

    // The listener effect subscribes once when isDragging flips true:
    // mousemove + mouseup + mouseleave === 3 adds.
    const addsAfterStart = countDragAdds()
    expect(addsAfterStart).toBe(3)

    // Fire many mousemoves. Each calls setDragState (mousePosition), which used
    // to re-run the effect and re-add all three listeners. With the fix the
    // count must NOT grow.
    act(() => {
      for (let i = 0; i < 20; i++) {
        document.dispatchEvent(mouseEvent('mousemove', 20 + i, 20 + i))
      }
    })

    expect(countDragAdds()).toBe(addsAfterStart)
    // And nothing was torn down mid-drag.
    const dragRemoves = removeSpy.mock.calls.filter(
      ([type]) => type === 'mousemove' || type === 'mouseup' || type === 'mouseleave',
    ).length
    expect(dragRemoves).toBe(0)
  })

  it('still handles mouseup after many mousemoves (no stranded isDragging)', () => {
    const { result } = renderHook(() => useFileExplorerDragDrop(undefined))

    act(() => {
      const e = {
        preventDefault: vi.fn(),
        stopPropagation: vi.fn(),
        clientX: 10,
        clientY: 10,
      } as unknown as React.MouseEvent
      result.current.startDrag(e, makeFile('a.txt', '/repo/a.txt'))
    })
    expect(result.current.dragState.isDragging).toBe(true)

    act(() => {
      for (let i = 0; i < 30; i++) {
        document.dispatchEvent(mouseEvent('mousemove', 30 + i, 30 + i))
      }
    })

    // mouseup over empty space (no drop target) must still end the drag.
    act(() => {
      document.dispatchEvent(mouseEvent('mouseup', 5, 5))
    })

    expect(result.current.dragState.isDragging).toBe(false)
    expect(result.current.dragState.draggedItem).toBeNull()
  })

  it('removes the document.body drag-preview div once the drag ends', () => {
    const { result } = renderHook(() => useFileExplorerDragDrop(undefined))

    act(() => {
      const e = {
        preventDefault: vi.fn(),
        stopPropagation: vi.fn(),
        clientX: 10,
        clientY: 10,
      } as unknown as React.MouseEvent
      result.current.startDrag(e, makeFile('a.txt', '/repo/a.txt'))
    })

    // Preview div is appended while dragging.
    expect(document.body.querySelectorAll('div').length).toBeGreaterThan(0)

    act(() => {
      document.dispatchEvent(mouseEvent('mouseup', 5, 5))
    })

    // After drop, no stranded fixed-position preview div on body.
    const stranded = Array.from(document.body.querySelectorAll('div')).filter(
      (el) => (el as HTMLElement).style.position === 'fixed',
    )
    expect(stranded.length).toBe(0)
  })
})
