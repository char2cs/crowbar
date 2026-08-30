/**
 * `useSidebarDrag`, driven directly — without `SidebarTree`/`RecentsBand`
 * around it.
 *
 * Mirrors the harness the two predecessor hooks (`workspace-tree-context`'s
 * drag half, `use-agent-chats-drag`) were tested with before their retirement
 * (commit f119a402): a fake `document.elementsFromPoint` answered from real
 * rows stubbed into the DOM, hand-driven `requestAnimationFrame` for the edge
 * scroller, and `press`/`move`/`release` helpers dispatching the same window
 * `pointermove`/`pointerup` events the hook itself listens for.
 */
import { act, cleanup, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from 'vitest'
import { createRef } from 'react'
import {
  useSidebarDrag,
  SIDEBAR_DRAG_THRESHOLD_PX,
  type SidebarPaneZone,
} from '@/components/sidebar/hooks/use-sidebar-drag'
import { PANE_DROP_ATTR } from '@/components/layout/drop-target-dom'
import { getInitialState, useSidebarStore } from '@/lib/store/sidebar'
import type { DropMode } from '@/components/tree-dnd/drop-core'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

const ROW_H = 36

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

/** A row in the tree, published exactly as `useSidebarDrag`'s own row spec
 *  reads it back — hand-built rather than routed through a rendered
 *  `<SidebarRow>`, since this suite is exercising the hook in isolation. */
function makeRow(
  row: SidebarRow,
  index: number,
  extra: { path?: string; expanded?: boolean; hasChildren?: boolean } = {},
) {
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

function makePane(paneId: string, rect: Partial<DOMRect>) {
  const el = document.createElement('div')
  el.setAttribute(PANE_DROP_ATTR, paneId)
  stubRect(el, rect)
  document.body.appendChild(el)
  return el
}

/** Answer the shared hit test from whatever is actually in the document,
 *  topmost (last-appended) first — mirrors real `elementsFromPoint` order. */
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

// Typed to the hook's own callbacks: a bare `vi.fn()` is a mock of anything,
// which the options object then refuses.
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
      subjectsFor: overrides.subjectsFor ?? (() => [baseRow]),
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

function move(x: number, y: number) {
  act(() => {
    window.dispatchEvent(new MouseEvent('pointermove', { clientX: x, clientY: y, bubbles: true }))
  })
}

function release(x: number, y: number) {
  act(() => {
    window.dispatchEvent(new MouseEvent('pointerup', { clientX: x, clientY: y, bubbles: true }))
  })
}

beforeEach(() => {
  Element.prototype.setPointerCapture = () => {}
  vi.stubGlobal('requestAnimationFrame', () => 0)
  vi.stubGlobal('cancelAnimationFrame', () => {})
  stubHitTest()
  // SIDEBAR_DROP_POLICY resolves scope against the LIVE store (never against
  // a row's own fields), so every id this suite drags or targets needs a
  // real entry here — one repo, so every pairing below is same-repo/
  // same-project and the matrix's own refusals are never what is under test.
  useSidebarStore.setState({
    ...getInitialState(),
    repos: [
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
    ],
  })
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

  it('a press past the threshold starts a drag', () => {
    const rowA = makeRow(baseRow, 0)
    const { result } = renderDrag()

    press(result, baseRow, rowA)
    move(10 + SIDEBAR_DRAG_THRESHOLD_PX + 1, 10)

    expect(result.current.dragging).toBe(true)
    expect(result.current.draggingIds.has('a')).toBe(true)
    release(10, 10)
  })

  it('drops onto a row: onDrop fires with the subjects, target and resolved mode', () => {
    const target: SidebarRow = { ...baseRow, id: 'b', label: 'b' }
    const rowA = makeRow(baseRow, 0)
    makeRow(target, 1)
    const { result, onDrop } = renderDrag()

    press(result, baseRow, rowA)
    move(10, ROW_H + 2) // top band of row b → 'before'
    release(10, ROW_H + 2)

    expect(onDrop).toHaveBeenCalledTimes(1)
    const [subjects, hitTarget, mode] = onDrop.mock.calls[0]
    expect(subjects).toEqual([baseRow])
    expect(hitTarget).toMatchObject({ id: 'b' })
    expect(mode).toBe('before')
    expect(result.current.dragging).toBe(false)
  })

  it('dropping on the middle third of a pane calls onPaneDrop with zone center', () => {
    const rowA = makeRow(baseRow, 0)
    makePane('pane-1', { top: 100, bottom: 300, left: 300, right: 500, width: 200, height: 200 })
    const { result, onPaneDrop } = renderDrag()

    press(result, baseRow, rowA)
    move(400, 200) // dead centre of the pane's rect
    release(400, 200)

    expect(onPaneDrop).toHaveBeenCalledWith([baseRow], 'pane-1', 'center')
  })

  it('dropping on the edge of a pane calls onPaneDrop with a side zone', () => {
    const rowA = makeRow(baseRow, 0)
    makePane('pane-1', { top: 100, bottom: 300, left: 300, right: 500, width: 200, height: 200 })
    const { result, onPaneDrop } = renderDrag()

    press(result, baseRow, rowA)
    move(310, 200) // left edge band
    release(310, 200)

    expect(onPaneDrop).toHaveBeenCalledWith([baseRow], 'pane-1', 'left')
  })

  it('refuses a drop onto the dragged row’s own descendant, via its published path', () => {
    // 'a' is dragged; 'grandchild' publishes an ancestor chain running through it.
    const rowA = makeRow(baseRow, 0)
    makeRow({ ...baseRow, id: 'grandchild', label: 'grandchild' }, 1, { path: '/root/a/' })
    const { result, onDrop } = renderDrag()

    press(result, baseRow, rowA)
    move(10, ROW_H + 2)
    release(10, ROW_H + 2)

    expect(onDrop).not.toHaveBeenCalled()
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

  it('a working row is refused everywhere — the matrix carries through to the hit test', () => {
    const workingRow: SidebarRow = { ...baseRow, working: true }
    const rowA = makeRow(workingRow, 0)
    makeRow({ ...baseRow, id: 'b', label: 'b' }, 1)
    const { result, onDrop } = renderDrag({ subjectsFor: () => [workingRow] })

    press(result, workingRow, rowA)
    move(10, ROW_H + 2)
    release(10, ROW_H + 2)

    expect(onDrop).not.toHaveBeenCalled()
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
})
