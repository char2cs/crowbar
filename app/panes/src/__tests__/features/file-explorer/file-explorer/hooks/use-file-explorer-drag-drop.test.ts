import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { MockInstance } from 'vitest'
import { useFileExplorerDragDrop } from '@/features/file-explorer/file-explorer/hooks/use-file-explorer-drag-drop'
import type { FileEntry } from '@/features/file-system/types/app'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { useDragStore } from '@/features/panes/stores/drag-store'
import { workspaceRef } from '@/lib/api/documents'
import { moveFile } from '@/features/file-system/controllers/platform'

vi.mock('@/features/file-system/controllers/platform', () => ({
  moveFile: vi.fn().mockResolvedValue(undefined),
}))

const { openDocument } = vi.hoisted(() => ({
  openDocument: vi.fn(async () => null),
}))
vi.mock('@/features/editor/documents/open-document', () => ({ openDocument }))

const makeFile = (name: string, path: string, isDir = false): FileEntry => ({
  name,
  path,
  isDir,
})

const mouseEvent = (type: string, x = 50, y = 50) =>
  new MouseEvent(type, { clientX: x, clientY: y, bubbles: true })

describe('useFileExplorerDragDrop — stable listener subscription', () => {
  let addSpy: MockInstance<typeof document.addEventListener>
  let removeSpy: MockInstance<typeof document.removeEventListener>

  beforeEach(() => {
    addSpy = vi.spyOn(document, 'addEventListener')
    removeSpy = vi.spyOn(document, 'removeEventListener')
    ;(document as unknown as { elementFromPoint: () => Element | null }).elementFromPoint = () =>
      null
  })

  afterEach(() => {
    addSpy.mockRestore()
    removeSpy.mockRestore()
    vi.clearAllMocks()
    delete (document as unknown as { elementFromPoint?: unknown }).elementFromPoint
  })

  const countDragAdds = () =>
    addSpy.mock.calls.filter(
      ([type]) => type === 'mousemove' || type === 'mouseup' || type === 'mouseleave',
    ).length

  it('attaches each document listener exactly once per drag, not once per mousemove', () => {
    const { result } = renderHook(() => useFileExplorerDragDrop('ws-a', undefined))

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

    const addsAfterStart = countDragAdds()
    expect(addsAfterStart).toBe(3)

    act(() => {
      for (let i = 0; i < 20; i++) {
        document.dispatchEvent(mouseEvent('mousemove', 20 + i, 20 + i))
      }
    })

    expect(countDragAdds()).toBe(addsAfterStart)
    const dragRemoves = removeSpy.mock.calls.filter(
      ([type]) => type === 'mousemove' || type === 'mouseup' || type === 'mouseleave',
    ).length
    expect(dragRemoves).toBe(0)
  })

  it('still handles mouseup after many mousemoves (no stranded isDragging)', () => {
    const { result } = renderHook(() => useFileExplorerDragDrop('ws-a', undefined))

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

    act(() => {
      document.dispatchEvent(mouseEvent('mouseup', 5, 5))
    })

    expect(result.current.dragState.isDragging).toBe(false)
    expect(result.current.dragState.draggedItem).toBeNull()
  })

  it('removes the document.body drag-preview div once the drag ends', () => {
    const { result } = renderHook(() => useFileExplorerDragDrop('ws-a', undefined))

    act(() => {
      const e = {
        preventDefault: vi.fn(),
        stopPropagation: vi.fn(),
        clientX: 10,
        clientY: 10,
      } as unknown as React.MouseEvent
      result.current.startDrag(e, makeFile('a.txt', '/repo/a.txt'))
    })

    expect(document.body.querySelectorAll('div').length).toBeGreaterThan(0)

    act(() => {
      document.dispatchEvent(mouseEvent('mouseup', 5, 5))
    })

    const stranded = Array.from(document.body.querySelectorAll('div')).filter(
      (el) => (el as HTMLElement).style.position === 'fixed',
    )
    expect(stranded.length).toBe(0)
  })

  it('paints the preview on the raised control surface, which exists in every theme', () => {
    const { result } = renderHook(() => useFileExplorerDragDrop('ws-a', undefined))

    act(() => {
      const e = {
        preventDefault: vi.fn(),
        stopPropagation: vi.fn(),
        clientX: 10,
        clientY: 10,
      } as unknown as React.MouseEvent
      result.current.startDrag(e, makeFile('a.txt', '/repo/a.txt'))
    })

    const preview = Array.from(document.body.querySelectorAll<HTMLElement>('div')).find(
      (el) => el.style.position === 'fixed',
    )
    expect(preview?.style.backgroundColor).toBe('var(--control-surface)')
    expect(preview?.style.color).toBe('var(--control-foreground)')
    expect(preview).toHaveAttribute('data-drag-overlay')
    expect(preview?.style.border).toContain('var(--border)')

    act(() => {
      document.dispatchEvent(mouseEvent('mouseup', 5, 5))
    })
  })
})

describe('useFileExplorerDragDrop — dropping a file on a pane', () => {
  let pane: HTMLElement

  beforeEach(() => {
    pane = document.createElement('div')
    pane.setAttribute('data-pane-container', '')
    pane.dataset.paneId = 'pane-b'
    document.body.appendChild(pane)
    ;(document as unknown as { elementFromPoint: () => Element | null }).elementFromPoint = () =>
      pane
    ;(document as unknown as { elementsFromPoint: () => Element[] }).elementsFromPoint = () => [
      pane,
    ]
  })

  afterEach(() => {
    pane.remove()
    setActiveWorkspaceId(null)
    openDocument.mockClear()
    delete (document as unknown as { elementFromPoint?: unknown }).elementFromPoint
    delete (document as unknown as { elementsFromPoint?: unknown }).elementsFromPoint
  })

  function drag(file: FileEntry) {
    const { result } = renderHook(() => useFileExplorerDragDrop('ws-a', undefined))
    act(() => {
      result.current.startDrag(
        {
          preventDefault: vi.fn(),
          stopPropagation: vi.fn(),
          clientX: 10,
          clientY: 10,
        } as unknown as React.MouseEvent,
        file,
      )
    })
    return result
  }

  it('opens the file of its own workspace in the pane it was dropped on, by id', () => {
    setActiveWorkspaceId('ws-other')
    const result = drag(makeFile('a.ts', 'src/a.ts'))
    expect(useDragStore.getState().file).toEqual({ path: 'src/a.ts', name: 'a.ts', isDir: false })

    act(() => {
      document.dispatchEvent(mouseEvent('mouseup', 1, 50))
    })

    expect(openDocument).toHaveBeenCalledWith(workspaceRef('ws-a', 'src/a.ts'), {
      paneId: 'pane-b',
    })
    expect(result.current.dragState.isDragging).toBe(false)
    expect(useDragStore.getState().file).toBeNull()
  })

  it('ignores a directory drop', () => {
    drag(makeFile('src', 'src', true))
    act(() => {
      document.dispatchEvent(mouseEvent('mouseup', 50, 50))
    })
    expect(openDocument).not.toHaveBeenCalled()
  })
})

describe('useFileExplorerDragDrop — moving a file within the tree', () => {
  let folder: HTMLElement

  beforeEach(() => {
    const tree = document.createElement('div')
    tree.className = 'file-tree-container'
    folder = document.createElement('div')
    folder.setAttribute('data-file-path', 'dst')
    folder.setAttribute('data-is-dir', 'true')
    tree.appendChild(folder)
    document.body.appendChild(tree)
    ;(document as unknown as { elementFromPoint: () => Element | null }).elementFromPoint = () =>
      folder
    setActiveWorkspaceId('ws-in-view')
  })

  afterEach(() => {
    document.body.replaceChildren()
    setActiveWorkspaceId(null)
    vi.mocked(moveFile).mockClear()
    delete (document as unknown as { elementFromPoint?: unknown }).elementFromPoint
  })

  it('moves within the explorer’s own workspace, not the one in view', async () => {
    const onFileMove = vi.fn()
    const { result } = renderHook(() => useFileExplorerDragDrop('ws-a', undefined, onFileMove))
    act(() => {
      result.current.startDrag(
        {
          preventDefault: vi.fn(),
          stopPropagation: vi.fn(),
          clientX: 10,
          clientY: 10,
        } as unknown as React.MouseEvent,
        makeFile('a.ts', 'src/a.ts'),
      )
    })
    act(() => {
      document.dispatchEvent(mouseEvent('mousemove', 20, 20))
    })

    await act(async () => {
      document.dispatchEvent(mouseEvent('mouseup', 20, 20))
    })

    expect(moveFile).toHaveBeenCalledExactlyOnceWith('ws-a', 'src/a.ts', 'dst/a.ts')
    expect(onFileMove).toHaveBeenCalledWith('src/a.ts', 'dst/a.ts')
  })
})
