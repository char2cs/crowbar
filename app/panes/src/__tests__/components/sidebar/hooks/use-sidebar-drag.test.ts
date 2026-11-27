import { act, cleanup, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from 'vitest'
import { createRef } from 'react'
import { useSidebarDrag } from '@/components/sidebar/hooks/use-sidebar-drag'
import { PANE_DROP_ATTR, type SidebarPaneZone } from '@/components/sidebar/lib/sidebar-drag-hover'
import { useDragStore } from '@/features/panes/stores/drag-store'
import { toast } from '@/features/window/stores/toast-store'
import type { DropMode } from '@/components/tree-dnd/drop-core'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { seedRepos } from '@/__tests__/__fixtures__/sidebar-entities'

const SIDEBAR_DRAG_THRESHOLD_PX = 8

vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: vi.fn(), info: vi.fn(), success: vi.fn() },
}))

const ROW_H = 36
const PANE_HIT_ATTR = 'data-pane-hit'

function stubRect(el: Element, rect: Partial<DOMRect>) {
  const full = { top: 0, left: 0, right: 0, bottom: 0, width: 0, height: 0, x: 0, y: 0, ...rect }
  el.getBoundingClientRect = () => ({ ...full, toJSON: () => full }) as DOMRect
}

const ROW_KIND_ATTR: Record<string, string> = {
  branch: 'data-sidebar-branch-drop',
  folder: 'data-sidebar-folder-drop',
  chat: 'data-sidebar-chat-drop',
  workflow: 'data-sidebar-workflow-drop',
}

const rowRegistry = new Map<string, SidebarRow>()

function makeRow(
  row: SidebarRow,
  index: number,
  extra: { path?: string; expanded?: boolean; hasChildren?: boolean } = {},
) {
  rowRegistry.set(row.id, row)
  const el = document.createElement('div')
  el.setAttribute(ROW_KIND_ATTR[row.kind], row.id)
  el.setAttribute('data-sidebar-drop-parent', row.parentId ?? '')
  if (extra.path !== undefined) el.setAttribute('data-sidebar-path', extra.path)
  if (extra.expanded) el.setAttribute('data-sidebar-expanded', '')
  if (extra.hasChildren) el.setAttribute('data-sidebar-children', '')
  stubRect(el, {
    top: index * ROW_H,
    bottom: (index + 1) * ROW_H,
    left: 0,
    right: 200,
    width: 200,
    height: ROW_H,
  })
  document.body.appendChild(el)
  return el
}

function paneBox(left: number): Partial<DOMRect> {
  return { top: 100, bottom: 300, left, right: left + 200, width: 200, height: 200 }
}

function makePane(paneId: string, rect: Partial<DOMRect>) {
  const el = document.createElement('div')
  el.setAttribute(PANE_DROP_ATTR, paneId)
  stubRect(el, rect)
  document.body.appendChild(el)
  return el
}

function stubHitTest() {
  document.elementsFromPoint = ((x: number, y: number) => {
    const hits: Element[] = []
    for (const el of document.querySelectorAll<HTMLElement>(
      `[${Object.values(ROW_KIND_ATTR).join('],[')}],[${PANE_DROP_ATTR}]`,
    )) {
      const r = el.getBoundingClientRect()
      if (r.width > 0 && x >= r.left && x <= r.right && y >= r.top && y < r.bottom) hits.push(el)
    }
    return hits.reverse()
  }) as typeof document.elementsFromPoint
}

const baseRow: SidebarRow = {
  id: 'a',
  kind: 'branch',
  parentId: null,
  order: 0,
  label: 'a',
  ownsWorktree: true,
  workspaceId: 'a',
  working: false,
  hasView: false,
}

type DropMock = Mock<(subjects: SidebarRow[], target: SidebarRow, mode: DropMode) => void>
type PaneDropMock = Mock<(subjects: SidebarRow[], paneId: string, zone: SidebarPaneZone) => void>

function renderDrag(
  overrides: {
    subjectsFor?: (rowId: string) => SidebarRow[]
    onDrop?: DropMock
    onPaneDrop?: PaneDropMock
    scroller?: HTMLElement | null
  } = {},
) {
  const onDrop: DropMock = overrides.onDrop ?? vi.fn()
  const onPaneDrop: PaneDropMock = overrides.onPaneDrop ?? vi.fn()
  const scrollRef = createRef<HTMLElement>() as { current: HTMLElement | null }
  scrollRef.current = overrides.scroller ?? null
  const view = renderHook(() =>
    useSidebarDrag({
      scrollRef,
      subjectsFor:
        overrides.subjectsFor ??
        ((rowId) => {
          const row = rowRegistry.get(rowId)
          return row ? [row] : []
        }),
      onDrop,
      onPaneDrop,
    }),
  )
  return { ...view, onDrop, onPaneDrop }
}

function press(
  hook: { current: ReturnType<typeof useSidebarDrag> },
  row: SidebarRow,
  target: HTMLElement,
  x = 10,
  y = 10,
) {
  act(() => {
    hook.current.onPointerDownDrag(row, {
      button: 0,
      clientX: x,
      clientY: y,
      pointerId: 1,
      currentTarget: target,
    } as unknown as React.PointerEvent)
  })
}

function move(x: number, y: number, buttons = 1) {
  act(() => {
    window.dispatchEvent(
      new MouseEvent('pointermove', { clientX: x, clientY: y, bubbles: true, buttons }),
    )
  })
}

function release(x: number, y: number) {
  act(() => {
    window.dispatchEvent(new MouseEvent('pointerup', { clientX: x, clientY: y, bubbles: true }))
  })
}

beforeEach(() => {
  rowRegistry.clear()
  vi.mocked(toast.error).mockClear()
  Element.prototype.setPointerCapture = () => {}
  vi.stubGlobal('requestAnimationFrame', () => 0)
  vi.stubGlobal('cancelAnimationFrame', () => {})
  stubHitTest()
  seedRepos([
    {
      id: 'repo-1',
      projectId: 'proj-1',
      name: 'repo-1',
      avatarLabel: 'R',
      avatarColor: 'bg-indigo-700',
      defaultWorkspaceId: 'home-1',
      workspaces: [
        { id: 'a', branch: 'a', age: '' },
        { id: 'b', branch: 'b', age: '' },
        { id: 'grandchild', branch: 'grandchild', age: '' },
        { id: 'ab', branch: 'ab', age: '' },
      ],
    },
  ])
})

afterEach(() => {
  cleanup()
  document.body.innerHTML = ''
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('useSidebarDrag', () => {
  it('a press under 5px does not start a drag', () => {
    const rowA = makeRow(baseRow, 0)
    const { result, onDrop } = renderDrag()

    press(result, baseRow, rowA)
    move(10 + SIDEBAR_DRAG_THRESHOLD_PX - 1, 10)

    expect(result.current.dragging).toBe(false)
    expect(onDrop).not.toHaveBeenCalled()
  })

  it('a press of EXACTLY the threshold does not start a drag — it is a strict "greater than"', () => {
    const rowA = makeRow(baseRow, 0)
    const { result, onDrop } = renderDrag()

    press(result, baseRow, rowA)
    move(10 + SIDEBAR_DRAG_THRESHOLD_PX, 10)

    expect(result.current.dragging).toBe(false)
    expect(onDrop).not.toHaveBeenCalled()
  })

  it('a drag still starts even if setPointerCapture throws', () => {
    const rowA = makeRow(baseRow, 0)
    Element.prototype.setPointerCapture = () => {
      throw new DOMException('no such pointer', 'NotFoundError')
    }
    const { result } = renderDrag()

    press(result, baseRow, rowA)
    move(10 + SIDEBAR_DRAG_THRESHOLD_PX + 1, 10)

    expect(result.current.dragging).toBe(true)
    release(10 + SIDEBAR_DRAG_THRESHOLD_PX + 1, 10)
  })

  it('a press past the threshold starts a drag', () => {
    const rowA = makeRow(baseRow, 0)
    const { result } = renderDrag()

    press(result, baseRow, rowA)
    move(10 + SIDEBAR_DRAG_THRESHOLD_PX + 1, 10)

    expect(result.current.dragging).toBe(true)
    expect(result.current.draggingIds.has('a')).toBe(true)
    release(10, 10)
  })

  it('drops onto a row: onDrop fires with the subjects, the REAL target row, and the resolved mode', () => {
    const target: SidebarRow = {
      id: 'b',
      kind: 'branch',
      parentId: null,
      order: 7,
      label: 'b the real row',
      ownsWorktree: false,
      workspaceId: 'ws-b',
      working: false,
      hasView: true,
    }
    const rowA = makeRow(baseRow, 0)
    makeRow(target, 1)
    const { result, onDrop } = renderDrag()

    press(result, baseRow, rowA)
    move(10, ROW_H + 2)
    release(10, ROW_H + 2)

    expect(onDrop).toHaveBeenCalledTimes(1)
    const [subjects, hitTarget, mode] = onDrop.mock.calls[0]
    expect(subjects).toEqual([baseRow])
    expect(hitTarget).toEqual(target)
    expect(mode).toBe('before')
    expect(result.current.dragging).toBe(false)
  })

  it('drops a chat dead-centre on a folder row: the resolved mode is "into", not a reorder', () => {
    const chatSubject: SidebarRow = {
      id: 'chat-a',
      kind: 'chat',
      parentId: null,
      order: 0,
      label: 'chat a',
      ownsWorktree: false,
      workspaceId: 'ws-1',
      working: false,
      hasView: false,
    }
    const folderTarget: SidebarRow = {
      id: 'folder-1',
      kind: 'folder',
      parentId: null,
      order: 1,
      label: 'Notes',
      ownsWorktree: false,
      workspaceId: null,
      working: false,
      hasView: false,
    }
    const rowA = makeRow(chatSubject, 0)
    makeRow(folderTarget, 1)
    const { result, onDrop } = renderDrag()

    press(result, chatSubject, rowA)
    move(10, ROW_H + ROW_H / 2)
    release(10, ROW_H + ROW_H / 2)

    expect(onDrop).toHaveBeenCalledTimes(1)
    const [subjects, hitTarget, mode] = onDrop.mock.calls[0]
    expect(subjects).toEqual([chatSubject])
    expect(hitTarget).toEqual(folderTarget)
    expect(mode).toBe('into')
  })

  it('refuses the drop rather than hand the caller a target it can no longer resolve', () => {
    const rowA = makeRow(baseRow, 0)
    makeRow({ ...baseRow, id: 'b', label: 'b' }, 1)
    rowRegistry.delete('b')
    const { result, onDrop } = renderDrag()

    press(result, baseRow, rowA)
    move(10, ROW_H + 2)
    release(10, ROW_H + 2)

    expect(onDrop).not.toHaveBeenCalled()
  })

  it('dropping on the middle third of a pane calls onPaneDrop with zone center', () => {
    const rowA = makeRow(baseRow, 0)
    makePane('pane-1', paneBox(300))
    const { result, onPaneDrop } = renderDrag()

    press(result, baseRow, rowA)
    move(400, 200)
    release(400, 200)

    expect(onPaneDrop).toHaveBeenCalledWith([baseRow], 'pane-1', 'center')
  })

  it('dropping on the edge of a pane calls onPaneDrop with a side zone', () => {
    const rowA = makeRow(baseRow, 0)
    makePane('pane-1', paneBox(300))
    const { result, onPaneDrop } = renderDrag()

    press(result, baseRow, rowA)
    move(310, 200)
    release(310, 200)

    expect(onPaneDrop).toHaveBeenCalledWith([baseRow], 'pane-1', 'left')
  })

  describe('the pane-hit indicator (spec §8.2)', () => {
    it('marks the hovered pane with PANE_HIT_ATTR, and only that pane', () => {
      const rowA = makeRow(baseRow, 0)
      const pane1 = makePane('pane-1', paneBox(300))
      const pane2 = makePane('pane-2', paneBox(600))
      const { result } = renderDrag()

      press(result, baseRow, rowA)
      move(400, 200)

      expect(pane1.hasAttribute(PANE_HIT_ATTR)).toBe(true)
      expect(pane2.hasAttribute(PANE_HIT_ATTR)).toBe(false)
    })

    it('moves the mark when the drag crosses from one pane to another', () => {
      const rowA = makeRow(baseRow, 0)
      const pane1 = makePane('pane-1', paneBox(300))
      const pane2 = makePane('pane-2', paneBox(600))
      const { result } = renderDrag()

      press(result, baseRow, rowA)
      move(400, 200)
      expect(pane1.hasAttribute(PANE_HIT_ATTR)).toBe(true)

      move(700, 200)
      expect(pane1.hasAttribute(PANE_HIT_ATTR)).toBe(false)
      expect(pane2.hasAttribute(PANE_HIT_ATTR)).toBe(true)
    })

    it('does not repaint on a zone change within the SAME pane (center → edge)', () => {
      const rowA = makeRow(baseRow, 0)
      const pane1 = makePane('pane-1', paneBox(300))
      const { result } = renderDrag()

      press(result, baseRow, rowA)
      move(400, 200)
      expect(pane1.hasAttribute(PANE_HIT_ATTR)).toBe(true)

      move(310, 200)
      expect(pane1.hasAttribute(PANE_HIT_ATTR)).toBe(true)
    })

    it('clears the mark once the pointer leaves every pane', () => {
      const rowA = makeRow(baseRow, 0)
      const pane1 = makePane('pane-1', paneBox(300))
      const { result } = renderDrag()

      press(result, baseRow, rowA)
      move(400, 200)
      expect(pane1.hasAttribute(PANE_HIT_ATTR)).toBe(true)

      move(10, ROW_H + 2)
      expect(pane1.hasAttribute(PANE_HIT_ATTR)).toBe(false)
    })

    it('clears the mark on release', () => {
      const rowA = makeRow(baseRow, 0)
      const pane1 = makePane('pane-1', paneBox(300))
      const { result } = renderDrag()

      press(result, baseRow, rowA)
      move(400, 200)
      release(400, 200)

      expect(pane1.hasAttribute(PANE_HIT_ATTR)).toBe(false)
    })

    it('clears the mark on pointercancel', () => {
      const rowA = makeRow(baseRow, 0)
      const pane1 = makePane('pane-1', paneBox(300))
      const { result } = renderDrag()

      press(result, baseRow, rowA)
      move(400, 200)
      act(() => {
        window.dispatchEvent(new Event('pointercancel', { bubbles: true }))
      })

      expect(pane1.hasAttribute(PANE_HIT_ATTR)).toBe(false)
    })

    it('marks the element the pointer is actually over — not the first same-paneId node in the document', () => {
      const rowA = makeRow(baseRow, 0)
      const hiddenOtherWorkspacePane = makePane('root-pane', {
        top: 0,
        bottom: 0,
        left: 0,
        right: 0,
        width: 0,
        height: 0,
      })
      const visiblePane = makePane('root-pane', paneBox(300))
      const { result } = renderDrag()

      press(result, baseRow, rowA)
      move(400, 200)

      expect(visiblePane.hasAttribute(PANE_HIT_ATTR)).toBe(true)
      expect(hiddenOtherWorkspacePane.hasAttribute(PANE_HIT_ATTR)).toBe(false)
    })
  })

  describe('the split preview a pane hover publishes (spec §8.1)', () => {
    const pane1Rect = {
      top: 100,
      bottom: 300,
      left: 300,
      right: 500,
      width: 200,
      height: 200,
    }

    it('names the pane AND the zone a release would land in', () => {
      const rowA = makeRow(baseRow, 0)
      makePane('pane-1', pane1Rect)
      const { result } = renderDrag()

      press(result, baseRow, rowA)
      move(400, 200)

      expect(useDragStore.getState().hover).toEqual({ paneId: 'pane-1', zone: 'center' })
    })

    it('follows the pointer from the middle out to an edge, within one pane', () => {
      const rowA = makeRow(baseRow, 0)
      makePane('pane-1', pane1Rect)
      const { result } = renderDrag()

      press(result, baseRow, rowA)
      move(400, 200)
      move(480, 200)

      expect(useDragStore.getState().hover).toEqual({ paneId: 'pane-1', zone: 'right' })

      move(400, 120)
      expect(useDragStore.getState().hover).toEqual({ paneId: 'pane-1', zone: 'top' })
    })

    it('clears once the pointer leaves every pane', () => {
      const rowA = makeRow(baseRow, 0)
      makePane('pane-1', pane1Rect)
      const { result } = renderDrag()

      press(result, baseRow, rowA)
      move(400, 200)
      move(10, ROW_H + 2)

      expect(useDragStore.getState().hover).toEqual({ paneId: null, zone: null })
    })

    it('clears on release, so the preview never outlives the drag that drew it', () => {
      const rowA = makeRow(baseRow, 0)
      makePane('pane-1', pane1Rect)
      const { result } = renderDrag()

      press(result, baseRow, rowA)
      move(400, 200)
      release(400, 200)

      expect(useDragStore.getState().hover).toEqual({ paneId: null, zone: null })
    })
  })

  it('refuses a drop onto the dragged row’s own descendant, via its published path', () => {
    const rowA = makeRow(baseRow, 0)
    makeRow({ ...baseRow, id: 'grandchild', label: 'grandchild' }, 1, { path: '/root/a/' })
    const { result, onDrop } = renderDrag()

    press(result, baseRow, rowA)
    move(10, ROW_H + 2)
    release(10, ROW_H + 2)

    expect(onDrop).not.toHaveBeenCalled()
  })

  it('surfaces a toast when the hit test refuses the ONLY row under the release point, not silence', () => {
    const rowA = makeRow(baseRow, 0)
    makeRow({ ...baseRow, id: 'grandchild', label: 'grandchild' }, 1, { path: '/root/a/' })
    const { result, onDrop } = renderDrag()

    press(result, baseRow, rowA)
    move(10, ROW_H + 2)
    release(10, ROW_H + 2)

    expect(onDrop).not.toHaveBeenCalled()
    expect(toast.error).toHaveBeenCalledWith("Can't move a there")
  })

  it('does not toast when the release lands back on the dragged row itself', () => {
    const rowA = makeRow(baseRow, 0)
    const { result, onDrop } = renderDrag()

    press(result, baseRow, rowA)
    move(10, ROW_H + 2)
    move(10, 3)
    release(10, 3)

    expect(onDrop).not.toHaveBeenCalled()
    expect(toast.error).not.toHaveBeenCalled()
  })

  it('does not refuse on a false substring match — a sibling path is not an ancestry hit', () => {
    const rowA = makeRow(baseRow, 0)
    makeRow({ ...baseRow, id: 'ab', label: 'ab' }, 1, { path: '/ab/' })
    const { result, onDrop } = renderDrag()

    press(result, baseRow, rowA)
    move(10, ROW_H + 2)
    release(10, ROW_H + 2)

    expect(onDrop).toHaveBeenCalledTimes(1)
  })

  it('a working row arms a drag like any other — pickup is no longer refused', () => {
    const workingRow: SidebarRow = { ...baseRow, working: true }
    const rowA = makeRow(workingRow, 0)
    const { result } = renderDrag({ subjectsFor: () => [workingRow] })

    press(result, workingRow, rowA)
    move(10, ROW_H + 2)

    expect(result.current.dragging).toBe(true)
    expect(result.current.ghostRows).not.toBeNull()
    release(10, ROW_H + 2)
  })

  it('a working chat dropped onto a pane still opens a split', () => {
    const workingRow: SidebarRow = { ...baseRow, working: true }
    const rowA = makeRow(workingRow, 0)
    makePane('pane-1', paneBox(300))
    const { result, onPaneDrop } = renderDrag({ subjectsFor: () => [workingRow] })

    press(result, workingRow, rowA)
    move(400, 200)
    release(400, 200)

    expect(onPaneDrop).toHaveBeenCalledWith([workingRow], 'pane-1', 'center')
  })

  it('a working chat dropped onto another row is still refused, with the same toast as any other refused reorder', () => {
    const workingRow: SidebarRow = { ...baseRow, working: true }
    const rowA = makeRow(workingRow, 0)
    makeRow({ ...baseRow, id: 'b', label: 'b' }, 1)
    const { result, onDrop } = renderDrag({ subjectsFor: () => [workingRow] })

    press(result, workingRow, rowA)
    move(10, ROW_H + 2)
    release(10, ROW_H + 2)

    expect(onDrop).not.toHaveBeenCalled()
    expect(toast.error).toHaveBeenCalledWith("Can't move a there")
  })

  describe('wheel-lock during a drag', () => {
    it('prevents wheel scrolling of the list while a drag is in flight', () => {
      const rowA = makeRow(baseRow, 0)
      const scroller = document.createElement('div')
      document.body.appendChild(scroller)
      const { result } = renderDrag({ scroller })

      press(result, baseRow, rowA)
      move(10, ROW_H + 2)
      expect(result.current.dragging).toBe(true)

      const wheelEvent = new WheelEvent('wheel', { cancelable: true })
      scroller.dispatchEvent(wheelEvent)
      expect(wheelEvent.defaultPrevented).toBe(true)

      release(10, ROW_H + 2)
    })

    it('stops blocking the wheel once the drag ends', () => {
      const rowA = makeRow(baseRow, 0)
      const scroller = document.createElement('div')
      document.body.appendChild(scroller)
      const { result } = renderDrag({ scroller })

      press(result, baseRow, rowA)
      move(10, ROW_H + 2)
      release(10, ROW_H + 2)

      const wheelEvent = new WheelEvent('wheel', { cancelable: true })
      scroller.dispatchEvent(wheelEvent)
      expect(wheelEvent.defaultPrevented).toBe(false)
    })

    it('never armed at all when the press never becomes a drag', () => {
      const rowA = makeRow(baseRow, 0)
      const scroller = document.createElement('div')
      document.body.appendChild(scroller)
      const { result } = renderDrag({ scroller })

      press(result, baseRow, rowA)
      move(10 + SIDEBAR_DRAG_THRESHOLD_PX - 1, 10)
      expect(result.current.dragging).toBe(false)

      const wheelEvent = new WheelEvent('wheel', { cancelable: true })
      scroller.dispatchEvent(wheelEvent)
      expect(wheelEvent.defaultPrevented).toBe(false)
    })
  })

  it('clears dragging state and the ghost after a release', () => {
    const rowA = makeRow(baseRow, 0)
    const target: SidebarRow = { ...baseRow, id: 'b', label: 'b' }
    makeRow(target, 1)
    const { result } = renderDrag()

    press(result, baseRow, rowA)
    move(10, ROW_H + 2)
    expect(result.current.dragging).toBe(true)
    expect(result.current.ghostRows).not.toBeNull()

    release(10, ROW_H + 2)
    expect(result.current.dragging).toBe(false)
    expect(result.current.ghostRows).toBeNull()
    expect(result.current.draggingIds.size).toBe(0)
    expect(result.current.nestTargetId).toBeNull()
    expect(result.current.paneHit).toBeNull()
  })

  it('clones the ACTUAL pressed element, not the first DOM node sharing its id', () => {
    const treeCopy = makeRow(baseRow, 0)
    treeCopy.setAttribute('data-test-marker', 'tree')
    const recentsCopy = makeRow(baseRow, 5)
    recentsCopy.setAttribute('data-test-marker', 'recents')
    const { result } = renderDrag()

    press(result, baseRow, recentsCopy)
    move(10, 5 * ROW_H + SIDEBAR_DRAG_THRESHOLD_PX + 1)

    expect(result.current.ghostRows).not.toBeNull()
    const clonedNode = result.current.ghostRows!.nodes[0]
    expect(clonedNode.getAttribute('data-test-marker')).toBe('recents')
    release(10, 5 * ROW_H + SIDEBAR_DRAG_THRESHOLD_PX + 1)
  })

  it('a pointercancel ends the drag without committing anything', () => {
    const rowA = makeRow(baseRow, 0)
    makeRow({ ...baseRow, id: 'b', label: 'b' }, 1)
    const { result, onDrop, onPaneDrop } = renderDrag()

    press(result, baseRow, rowA)
    move(10, ROW_H + 2)
    act(() => {
      window.dispatchEvent(new Event('pointercancel', { bubbles: true }))
    })

    expect(result.current.dragging).toBe(false)
    expect(onDrop).not.toHaveBeenCalled()
    expect(onPaneDrop).not.toHaveBeenCalled()
  })

  describe('selectstart guard recovery', () => {
    function selectstartBlocked(): boolean {
      const ev = new Event('selectstart', { cancelable: true })
      document.dispatchEvent(ev)
      return ev.defaultPrevented
    }

    it('a normal press+release arms the guard and then clears it', () => {
      const rowA = makeRow(baseRow, 0)
      const { result } = renderDrag()

      press(result, baseRow, rowA)
      expect(selectstartBlocked()).toBe(true)
      release(10, 10)
      expect(selectstartBlocked()).toBe(false)
    })

    it('a pointermove reporting the button already up ends an in-flight drag and clears the guard', () => {
      const rowA = makeRow(baseRow, 0)
      const { result } = renderDrag()

      press(result, baseRow, rowA)
      move(10, ROW_H + 2)
      expect(result.current.dragging).toBe(true)

      move(10, ROW_H + 2, 0)

      expect(result.current.dragging).toBe(false)
      expect(selectstartBlocked()).toBe(false)
    })

    it('a pointermove reporting the button already up before the drag threshold clears the pending press and the guard', () => {
      const rowA = makeRow(baseRow, 0)
      const { result } = renderDrag()

      press(result, baseRow, rowA)
      move(10, 10, 0)

      expect(result.current.dragging).toBe(false)
      expect(selectstartBlocked()).toBe(false)
    })

    it('the window losing focus mid-drag ends it and clears the guard', () => {
      const rowA = makeRow(baseRow, 0)
      const { result } = renderDrag()

      press(result, baseRow, rowA)
      move(10, ROW_H + 2)
      expect(result.current.dragging).toBe(true)

      act(() => {
        window.dispatchEvent(new Event('blur'))
      })

      expect(result.current.dragging).toBe(false)
      expect(selectstartBlocked()).toBe(false)
    })

    it('the window losing focus while still pending (below threshold) clears the guard', () => {
      const rowA = makeRow(baseRow, 0)
      const { result } = renderDrag()

      press(result, baseRow, rowA)
      act(() => {
        window.dispatchEvent(new Event('blur'))
      })

      expect(result.current.dragging).toBe(false)
      expect(selectstartBlocked()).toBe(false)
    })
  })

  it('dragProps publishes the row’s kind attribute and container', () => {
    const { result } = renderDrag()
    const props = result.current.dragProps(baseRow, {
      path: '/a/',
      expanded: true,
      hasChildren: false,
    })
    expect(props['data-sidebar-branch-drop']).toBe('a')
    expect(props['data-sidebar-drop-parent']).toBe('')
    expect(props['data-sidebar-path']).toBe('/a/')
    expect(props['data-sidebar-expanded']).toBe('')
    expect(props['data-sidebar-children']).toBeUndefined()
  })

  it('a stray element carrying the old trash-drop attribute is not treated as a drop target', () => {
    const rowA = makeRow(baseRow, 0)
    const stray = document.createElement('div')
    stray.setAttribute('data-sidebar-trash-drop', '')
    stubRect(stray, { top: 500, bottom: 600, left: 0, right: 300, width: 300, height: 100 })
    document.body.appendChild(stray)
    const { result, onDrop, onPaneDrop } = renderDrag()

    press(result, baseRow, rowA)
    move(10, 550)
    release(10, 550)

    expect(onDrop).not.toHaveBeenCalled()
    expect(onPaneDrop).not.toHaveBeenCalled()
  })

  it('carries every row the selection names, and hands them all to onDrop', () => {
    const rowA = makeRow(baseRow, 0)
    makeRow({ ...baseRow, id: 'b', label: 'b' }, 1)
    const second: SidebarRow = { ...baseRow, id: 'ab', label: 'ab', order: 2 }
    makeRow(second, 2)
    const { result, onDrop } = renderDrag({
      subjectsFor: (id) => (id === 'a' ? [baseRow, second] : [rowRegistry.get(id)!]),
    })

    press(result, baseRow, rowA)
    move(10, ROW_H + 2)
    expect([...result.current.draggingIds]).toEqual(['a', 'ab'])
    expect(result.current.ghostRows?.count).toBe(2)
    release(10, ROW_H + 2)

    expect(onDrop.mock.calls[0][0].map((r) => r.id)).toEqual(['a', 'ab'])
  })

  it('a pointer held at the list edge scrolls it and drops on the row that slid under it', () => {
    const frames: FrameRequestCallback[] = []
    vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => frames.push(cb))
    const scroller = document.createElement('div')
    let scrollTop = 0
    Object.defineProperty(scroller, 'scrollTop', {
      get: () => scrollTop,
      set: (v: number) => void (scrollTop = Math.max(0, Math.min(v, ROW_H))),
    })
    stubRect(scroller, { top: 0, bottom: 3 * ROW_H, height: 3 * ROW_H, right: 200, width: 200 })
    document.body.appendChild(scroller)
    const ids = ['a', 'b', 'ab', 'grandchild']
    const els = ids.map((id, i) => makeRow({ ...baseRow, id, label: id, order: i }, i))
    els.forEach((el, i) => {
      el.getBoundingClientRect = () => {
        const top = i * ROW_H - scrollTop
        return {
          top,
          bottom: top + ROW_H,
          left: 0,
          right: 200,
          width: 200,
          height: ROW_H,
        } as DOMRect
      }
    })
    const { result, onDrop } = renderDrag({ scroller })

    press(result, baseRow, els[0])
    move(10, 3 * ROW_H - 8)
    move(10, 3 * ROW_H - 8)
    for (let i = 0; i < 4; i++) act(() => frames.splice(0).forEach((cb) => cb(0)))
    release(10, 3 * ROW_H - 8)

    expect(scrollTop).toBe(ROW_H)
    expect(onDrop.mock.calls[0][1].id).toBe('grandchild')
  })
})
