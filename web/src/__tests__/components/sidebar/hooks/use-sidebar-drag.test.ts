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
  PANE_DROP_ATTR,
  PANE_HIT_ATTR,
  CARD_TRASH_DROP_ATTR,
  type SidebarPaneZone,
} from '@/components/sidebar/hooks/use-sidebar-drag'
import { getInitialState, useSidebarStore } from '@/lib/store/sidebar'
import { handleTrash } from '@/components/layout/space-content-actions'
import { toast } from '@/features/window/stores/toast-store'
import type { DropMode } from '@/components/tree-dnd/drop-core'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

// Addendum §2's drag-to-trash commits through the SAME `handleTrash`
// `space-content-actions.test.ts` already exercises in full (resolve →
// planRemoval → hold) — mocked here so this suite can assert the HOOK calls
// it with the right id(s) without also standing up a real sidebar/removal
// store fixture for every other test in this file.
vi.mock('@/components/layout/space-content-actions', () => ({
  handleTrash: vi.fn(() => true),
}))
vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: vi.fn(), info: vi.fn(), success: vi.fn() },
}))

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

/** Every real row this suite has ever `makeRow`'d, keyed by id — what the
 *  default `subjectsFor` below resolves against, mirroring how SidebarTree
 *  (`rows.find`) and RecentsBand (`rowsRef.current.get`) resolve a live row
 *  by id for real. `hit.row` off the DOM hit test is NOT a real `SidebarRow`
 *  (see use-sidebar-drag.ts's own onPointerUp comment) — the hook re-resolves
 *  the target through `subjectsFor` before calling `onDrop`, so a test that
 *  presses one row and drops onto another needs BOTH registered here, not
 *  just the one this suite happens to assert about. */
const rowRegistry = new Map<string, SidebarRow>()

/** A row in the tree, published exactly as `useSidebarDrag`'s own row spec
 *  reads it back — hand-built rather than routed through a rendered
 *  `<SidebarRow>`, since this suite is exercising the hook in isolation. */
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

function makePane(paneId: string, rect: Partial<DOMRect>) {
  const el = document.createElement('div')
  el.setAttribute(PANE_DROP_ATTR, paneId)
  stubRect(el, rect)
  document.body.appendChild(el)
  return el
}

/** The file explorer card's trash-target surface (addendum §2) — a bare
 *  presence flag, same shape as `makePane` but with no id to carry. */
function makeTrashZone(rect: Partial<DOMRect>) {
  const el = document.createElement('div')
  el.setAttribute(CARD_TRASH_DROP_ATTR, '')
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
      `[${Object.values(ROW_KIND_ATTR).join('],[')}],[${PANE_DROP_ATTR}],[${CARD_TRASH_DROP_ATTR}]`,
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
  rowRegistry.clear()
  vi.mocked(handleTrash).mockClear()
  vi.mocked(handleTrash).mockReturnValue(true)
  vi.mocked(toast.error).mockClear()
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

  it('a press of EXACTLY the threshold does not start a drag — it is a strict "greater than"', () => {
    const rowA = makeRow(baseRow, 0)
    const { result, onDrop } = renderDrag()

    press(result, baseRow, rowA)
    move(10 + SIDEBAR_DRAG_THRESHOLD_PX, 10)

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

  it('drops onto a row: onDrop fires with the subjects, the REAL target row, and the resolved mode', () => {
    // A real SidebarRow, not the partial shape the DOM hit test reconstructs
    // ({kind, id, parentId, path, expanded, hasChildren} — the fields the
    // matrix itself needs, nothing else): order/label/ownsWorktree/
    // workspaceId/working/hasView all differ from baseRow's on purpose, so a
    // regression that hands the caller the DOM-reconstructed stand-in
    // (missing every one of them, and 'parentId: ""' instead of `null`)
    // fails this on every extra field, not just `id`.
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
    move(10, ROW_H + 2) // top band of row b → 'before'
    release(10, ROW_H + 2)

    expect(onDrop).toHaveBeenCalledTimes(1)
    const [subjects, hitTarget, mode] = onDrop.mock.calls[0]
    expect(subjects).toEqual([baseRow])
    expect(hitTarget).toEqual(target)
    expect(mode).toBe('before')
    expect(result.current.dragging).toBe(false)
  })

  it('refuses the drop rather than hand the caller a target it can no longer resolve', () => {
    // A row can be hit-tested (it is still in the DOM) but no longer live in
    // the data `subjectsFor` resolves against — the same race `subjectsFor`
    // already refuses for on the SUBJECT side, now covered on the target
    // side too, now that onDrop's target is a real re-resolved row rather
    // than whatever the DOM hit test could reconstruct on its own.
    const rowA = makeRow(baseRow, 0)
    makeRow({ ...baseRow, id: 'b', label: 'b' }, 1)
    rowRegistry.delete('b') // still a real DOM row; no longer a resolvable one.
    const { result, onDrop } = renderDrag()

    press(result, baseRow, rowA)
    move(10, ROW_H + 2)
    release(10, ROW_H + 2)

    expect(onDrop).not.toHaveBeenCalled()
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

  // Fix round 1 (real, reviewer-verified gap): §8.2's "the entry about to
  // take a drop wears the same ring a pane wears" needs a PANE half too — a
  // neutral indicator marking which pane a release would land in. Painted
  // straight onto the DOM (`paintPaneHit`), reading the exact same
  // PANE_DROP_ATTR value the hit test itself resolves against.
  describe('the pane-hit indicator (spec §8.2)', () => {
    it('marks the hovered pane with PANE_HIT_ATTR, and only that pane', () => {
      const rowA = makeRow(baseRow, 0)
      const pane1 = makePane('pane-1', {
        top: 100,
        bottom: 300,
        left: 300,
        right: 500,
        width: 200,
        height: 200,
      })
      const pane2 = makePane('pane-2', {
        top: 100,
        bottom: 300,
        left: 600,
        right: 800,
        width: 200,
        height: 200,
      })
      const { result } = renderDrag()

      press(result, baseRow, rowA)
      move(400, 200) // over pane-1

      expect(pane1.hasAttribute(PANE_HIT_ATTR)).toBe(true)
      expect(pane2.hasAttribute(PANE_HIT_ATTR)).toBe(false)
    })

    it('moves the mark when the drag crosses from one pane to another', () => {
      const rowA = makeRow(baseRow, 0)
      const pane1 = makePane('pane-1', {
        top: 100,
        bottom: 300,
        left: 300,
        right: 500,
        width: 200,
        height: 200,
      })
      const pane2 = makePane('pane-2', {
        top: 100,
        bottom: 300,
        left: 600,
        right: 800,
        width: 200,
        height: 200,
      })
      const { result } = renderDrag()

      press(result, baseRow, rowA)
      move(400, 200) // pane-1
      expect(pane1.hasAttribute(PANE_HIT_ATTR)).toBe(true)

      move(700, 200) // pane-2
      expect(pane1.hasAttribute(PANE_HIT_ATTR)).toBe(false)
      expect(pane2.hasAttribute(PANE_HIT_ATTR)).toBe(true)
    })

    it('does not repaint on a zone change within the SAME pane (center → edge)', () => {
      const rowA = makeRow(baseRow, 0)
      const pane1 = makePane('pane-1', {
        top: 100,
        bottom: 300,
        left: 300,
        right: 500,
        width: 200,
        height: 200,
      })
      const { result } = renderDrag()

      press(result, baseRow, rowA)
      move(400, 200) // center
      expect(pane1.hasAttribute(PANE_HIT_ATTR)).toBe(true)

      move(310, 200) // left edge band, still pane-1
      expect(pane1.hasAttribute(PANE_HIT_ATTR)).toBe(true)
    })

    it('clears the mark once the pointer leaves every pane', () => {
      const rowA = makeRow(baseRow, 0)
      const pane1 = makePane('pane-1', {
        top: 100,
        bottom: 300,
        left: 300,
        right: 500,
        width: 200,
        height: 200,
      })
      const { result } = renderDrag()

      press(result, baseRow, rowA)
      move(400, 200)
      expect(pane1.hasAttribute(PANE_HIT_ATTR)).toBe(true)

      move(10, ROW_H + 2) // back over the tree, off every pane
      expect(pane1.hasAttribute(PANE_HIT_ATTR)).toBe(false)
    })

    it('clears the mark on release', () => {
      const rowA = makeRow(baseRow, 0)
      const pane1 = makePane('pane-1', {
        top: 100,
        bottom: 300,
        left: 300,
        right: 500,
        width: 200,
        height: 200,
      })
      const { result } = renderDrag()

      press(result, baseRow, rowA)
      move(400, 200)
      release(400, 200)

      expect(pane1.hasAttribute(PANE_HIT_ATTR)).toBe(false)
    })

    it('clears the mark on pointercancel', () => {
      const rowA = makeRow(baseRow, 0)
      const pane1 = makePane('pane-1', {
        top: 100,
        bottom: 300,
        left: 300,
        right: 500,
        width: 200,
        height: 200,
      })
      const { result } = renderDrag()

      press(result, baseRow, rowA)
      move(400, 200)
      act(() => {
        window.dispatchEvent(new Event('pointercancel', { bubbles: true }))
      })

      expect(pane1.hasAttribute(PANE_HIT_ATTR)).toBe(false)
    })

    // Fix round 2 (real, reviewer-verified regression in this same round's
    // new code): `WorkspaceHost` keeps every retained workspace mounted at
    // once — hidden via display:none, never unmounted — and each one's
    // `WorkspaceView` renders its own full pane tree regardless of whether
    // it's the active workspace. `ROOT_PANE_ID`/`BOTTOM_PANE_ID` are literal
    // constants every workspace store shares, so TWO elements on the page can
    // legitimately carry the identical `data-pane-drop="root-pane"` value —
    // one hidden/off-screen, one the pointer is actually over. The original
    // `paintPaneHit` re-resolved its target with
    // `document.querySelector('[data-pane-drop="…"]')`, which answers with
    // the FIRST document-order match regardless of visibility — exactly the
    // shared-pane-id-across-hidden-workspaces hazard the cross-workspace
    // commit fix (Fix round 1) already closed for `performSidebarPaneDrop`,
    // reopened here in the ring's own lookup. The fix threads the REAL
    // element `elementsFromPoint` resolved (`ResolvedPaneHit.el`) straight
    // through instead of re-deriving one by attribute.
    it('marks the element the pointer is actually over — not the first same-paneId node in the document', () => {
      const rowA = makeRow(baseRow, 0)
      // A hidden OTHER workspace's pane sharing the exact same paneId,
      // mounted FIRST (so it would win a `querySelector` lookup). Zero-size,
      // exactly as a real `display:none` node would be — the stubbed hit
      // test (elementsFromPoint) can therefore never resolve TO it, only
      // `document.querySelector` (the bug) could have.
      const hiddenOtherWorkspacePane = makePane('root-pane', {
        top: 0,
        bottom: 0,
        left: 0,
        right: 0,
        width: 0,
        height: 0,
      })
      // The VISIBLE, actually-hit pane — same paneId, mounted second.
      const visiblePane = makePane('root-pane', {
        top: 100,
        bottom: 300,
        left: 300,
        right: 500,
        width: 200,
        height: 200,
      })
      const { result } = renderDrag()

      press(result, baseRow, rowA)
      move(400, 200) // dead centre of the VISIBLE pane's rect

      expect(visiblePane.hasAttribute(PANE_HIT_ATTR)).toBe(true)
      expect(hiddenOtherWorkspacePane.hasAttribute(PANE_HIT_ATTR)).toBe(false)
    })
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

  // Spec §8.3: "a working chat may not be dragged." Refused at PICKUP now
  // (`onPointerDownDrag`), before a drag is ever armed — stronger than the
  // matrix's own working-subject refusal (`SIDEBAR_DROP_POLICY`), which
  // still exists but would only ever fire once a drag is already in the air.
  it('a working row never arms a drag at all — refused at pickup, not just at the hit test', () => {
    const workingRow: SidebarRow = { ...baseRow, working: true }
    const rowA = makeRow(workingRow, 0)
    makeRow({ ...baseRow, id: 'b', label: 'b' }, 1)
    const { result, onDrop } = renderDrag({ subjectsFor: () => [workingRow] })

    press(result, workingRow, rowA)
    move(10, ROW_H + 2)

    // Never even reached `dragging: true` — nothing to release.
    expect(result.current.dragging).toBe(false)
    expect(result.current.ghostRows).toBeNull()

    release(10, ROW_H + 2)
    expect(onDrop).not.toHaveBeenCalled()
  })

  describe('the working-drag refusal (spec §8.3)', () => {
    beforeEach(() => {
      vi.useFakeTimers()
    })
    afterEach(() => {
      vi.useRealTimers()
    })

    it('dims the scroller and paints the dragged row red, with a short note', () => {
      const workingRow: SidebarRow = { ...baseRow, working: true }
      const rowA = makeRow(workingRow, 0)
      const scroller = document.createElement('div')
      document.body.appendChild(scroller)
      const { result } = renderDrag({ subjectsFor: () => [workingRow], scroller })

      press(result, workingRow, rowA)

      expect(scroller.style.opacity).toBe('0.4')
      expect(rowA.style.outline).toContain('var(--destructive)')
      expect(document.querySelector('[data-row-drag-refusal]')).not.toBeNull()
      expect(document.querySelector('[data-row-drag-refusal]')?.textContent).toMatch(/working/i)
    })

    it('clears itself after the timeout, undoing every style it painted', () => {
      const workingRow: SidebarRow = { ...baseRow, working: true }
      const rowA = makeRow(workingRow, 0)
      const scroller = document.createElement('div')
      document.body.appendChild(scroller)
      const { result } = renderDrag({ subjectsFor: () => [workingRow], scroller })

      press(result, workingRow, rowA)
      expect(document.querySelector('[data-row-drag-refusal]')).not.toBeNull()

      act(() => {
        vi.advanceTimersByTime(2000)
      })

      expect(scroller.style.opacity).toBe('')
      expect(rowA.style.outline).toBe('')
      expect(document.querySelector('[data-row-drag-refusal]')).toBeNull()
    })

    it('clears early on the next pointerdown anywhere, not just on its own timer', () => {
      const workingRow: SidebarRow = { ...baseRow, working: true }
      const rowA = makeRow(workingRow, 0)
      const { result } = renderDrag({ subjectsFor: () => [workingRow] })

      press(result, workingRow, rowA)
      expect(document.querySelector('[data-row-drag-refusal]')).not.toBeNull()

      act(() => {
        window.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true }))
      })

      expect(document.querySelector('[data-row-drag-refusal]')).toBeNull()
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

  // Addendum §2: dropping a row on the file explorer card's trash surface
  // feeds `handleTrash` — the same removal-tray path a row's own trash
  // button (now gone, per addendum §1) used to call directly.
  describe('drag-to-trash (addendum §2)', () => {
    it('dropping a row on the trash zone calls handleTrash with its id', () => {
      const rowA = makeRow(baseRow, 0)
      makeTrashZone({ top: 500, bottom: 600, left: 0, right: 300, width: 300, height: 100 })
      const { result, onDrop, onPaneDrop } = renderDrag()

      press(result, baseRow, rowA)
      move(10, 550)
      release(10, 550)

      expect(handleTrash).toHaveBeenCalledExactlyOnceWith('a')
      // Never ALSO treated as a row or pane drop.
      expect(onDrop).not.toHaveBeenCalled()
      expect(onPaneDrop).not.toHaveBeenCalled()
    })

    it('the trash zone wins over a pane sitting behind it, and a pane wins when the trash zone is not there', () => {
      const rowA = makeRow(baseRow, 0)
      // Same screen region, both present — the trash zone is appended AFTER
      // the pane, so it paints on top and must be the one that wins.
      makePane('pane-1', { top: 500, bottom: 600, left: 0, right: 300, width: 300, height: 100 })
      const trash = makeTrashZone({
        top: 500,
        bottom: 600,
        left: 0,
        right: 300,
        width: 300,
        height: 100,
      })
      const { result, onPaneDrop } = renderDrag()

      press(result, baseRow, rowA)
      move(150, 550) // dead centre of the shared rect
      release(150, 550)

      expect(handleTrash).toHaveBeenCalledExactlyOnceWith('a')
      expect(onPaneDrop).not.toHaveBeenCalled()

      // Remove the trash zone — now the SAME pane underneath it is reachable.
      trash.remove()
      vi.mocked(handleTrash).mockClear()
      press(result, baseRow, rowA)
      move(150, 550)
      release(150, 550)

      expect(onPaneDrop).toHaveBeenCalledWith([baseRow], 'pane-1', 'center')
      expect(handleTrash).not.toHaveBeenCalled()
    })

    it('a working row never reaches the trash zone either — refused at pickup', () => {
      const workingRow: SidebarRow = { ...baseRow, working: true }
      const rowA = makeRow(workingRow, 0)
      makeTrashZone({ top: 500, bottom: 600, left: 0, right: 300, width: 300, height: 100 })
      const { result } = renderDrag({ subjectsFor: () => [workingRow] })

      press(result, workingRow, rowA)
      move(10, 550)
      release(10, 550)

      expect(handleTrash).not.toHaveBeenCalled()
    })

    it('a refused drop (locked branch, repo home — handleTrash returns false) surfaces a toast, not a silent no-op', () => {
      vi.mocked(handleTrash).mockReturnValue(false)
      const row: SidebarRow = { ...baseRow, label: 'develop' }
      const rowA = makeRow(row, 0)
      makeTrashZone({ top: 500, bottom: 600, left: 0, right: 300, width: 300, height: 100 })
      const { result } = renderDrag({ subjectsFor: () => [row] })

      press(result, row, rowA)
      move(10, 550)
      release(10, 550)

      expect(toast.error).toHaveBeenCalledWith("Can't delete develop — it may be locked")
    })

    it('drops every dragged subject, not just the first', () => {
      const rowA = makeRow(baseRow, 0)
      const rowB: SidebarRow = { ...baseRow, id: 'b', label: 'b' }
      makeRow(rowB, 1)
      makeTrashZone({ top: 500, bottom: 600, left: 0, right: 300, width: 300, height: 100 })
      const { result } = renderDrag({ subjectsFor: () => [baseRow, rowB] })

      press(result, baseRow, rowA)
      move(10, 550)
      release(10, 550)

      expect(handleTrash).toHaveBeenCalledTimes(2)
      expect(handleTrash).toHaveBeenCalledWith('a')
      expect(handleTrash).toHaveBeenCalledWith('b')
    })
  })
})
