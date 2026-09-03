/**
 * The chats drag, driven directly — without a panel around it.
 *
 * The panel's own tests cover the gesture as the user makes it. What they cannot
 * reach are the states where the drag's surroundings are INCOMPLETE: the frame
 * before the hairline has mounted, a row that left the DOM between the press and
 * the threshold, a panel whose scroller has not been measured. Every one of
 * those is a real frame in a real drag — React commits a render after the
 * pointer handler that started it, and a chat can be deleted on the wire at any
 * moment — and every one of them is a place where a missing guard is a thrown
 * exception in the middle of a gesture.
 *
 * Rendering the hook alone is what makes them reachable: nothing here mounts an
 * indicator or a ghost, so the drag runs with those refs empty for its whole
 * length.
 */
import { act, cleanup, render, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from 'vitest'
import { createRef } from 'react'
import {
  useAgentChatsDrag,
  CHAT_SPRING_OPEN_MS,
} from '@/features/agent/tree/hooks/use-agent-chats-drag'
import {
  EditorRemovalOverlay,
  PANE_ARM_MS,
  type RemovalOverlayText,
} from '@/components/layout/editor-removal-overlay'
import { PANE_DROP_ATTR } from '@/components/layout/drop-target-dom'
import {
  chatRowProps,
  type ChatDragSubject,
  type ResolvedChatDrop,
} from '@/features/agent/tree/lib/chat-drop'

// ── Harness ─────────────────────────────────────────────────────────

const ROW_H = 40

/** The frames the scroller has asked for, flushed on demand. */
let frames: FrameRequestCallback[] = []

function stubRect(el: Element, rect: Partial<DOMRect>) {
  const full = { top: 0, left: 0, right: 0, bottom: 0, width: 0, height: 0, x: 0, y: 0, ...rect }
  el.getBoundingClientRect = () => ({ ...full, toJSON: () => full }) as DOMRect
}

/** A row in the tree, published exactly as the real rows publish themselves. */
function makeRow(id: string, index: number, extra: { parentId?: string; path?: string } = {}) {
  const el = document.createElement('div')
  const props = chatRowProps({
    kind: 'chat',
    id,
    parentId: extra.parentId ?? '',
    path: extra.path ?? '/',
    expanded: true,
    hasChildren: false,
  })
  for (const [attr, value] of Object.entries(props)) {
    if (value !== undefined) el.setAttribute(attr, value)
  }
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

/** Answer the shared hit test from the rows that are actually in the document. */
function stubHitTest() {
  document.elementsFromPoint = ((_x: number, y: number) => {
    for (const el of document.querySelectorAll<HTMLElement>('[data-chat-drop]')) {
      const r = el.getBoundingClientRect()
      if (r.height > 0 && y >= r.top && y < r.bottom) return [el]
    }
    return []
  }) as typeof document.elementsFromPoint
}

// Typed to the hook's own callbacks: a bare `vi.fn()` is a mock of anything,
// which the options object then refuses.
type DropMock = Mock<(subjects: readonly ChatDragSubject[], target: ResolvedChatDrop) => void>
type SubjectsMock = Mock<(subjects: readonly ChatDragSubject[]) => void>
type IdMock = Mock<(id: string) => void>

type RemovalTextMock = Mock<
  (subjects: readonly ChatDragSubject[], armed: boolean) => RemovalOverlayText | null
>

interface Harness {
  scroller: HTMLElement | null
  onDrop: DropMock
  onPaneRemove: SubjectsMock
  removalText: RemovalTextMock
  onSpringOpen: IdMock
  selection: ChatDragSubject[]
}

/** What a panel with something to remove would answer. */
const someRemoval: RemovalTextMock = vi.fn((_subjects, armed: boolean) => ({
  title: armed ? 'Release to remove First' : 'Drop here to remove First',
  detail: 'You will have 8 seconds to undo',
  armed,
}))

function renderDrag(harness: Partial<Harness> = {}) {
  const onDrop: DropMock = harness.onDrop ?? vi.fn()
  const onPaneRemove: SubjectsMock = harness.onPaneRemove ?? vi.fn()
  const removalText: RemovalTextMock = harness.removalText ?? someRemoval
  const onSpringOpen: IdMock = harness.onSpringOpen ?? vi.fn()
  const scrollRef = createRef<HTMLDivElement>() as { current: HTMLDivElement | null }
  scrollRef.current = (harness.scroller ?? null) as HTMLDivElement | null
  const view = renderHook(() =>
    useAgentChatsDrag({
      scrollRef,
      subjectsFor: () => harness.selection ?? [],
      onDrop,
      onPaneRemove,
      removalText,
      onSpringOpen,
    }),
  )
  return { ...view, onDrop, onPaneRemove, removalText, onSpringOpen }
}

/** The press, as the row hands it over. */
function press(
  hook: { current: ReturnType<typeof useAgentChatsDrag> },
  subject: ChatDragSubject,
  target: HTMLElement,
  x = 10,
  y = 10,
) {
  act(() => {
    hook.current.onPointerDownDrag(subject, {
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

/** Run whatever the scroller asked for, one frame at a time. */
function flushFrame() {
  const pending = frames
  frames = []
  act(() => {
    for (const cb of pending) cb(0)
  })
}

const CHAT_A: ChatDragSubject = { kind: 'chat', id: 'a', parentId: '' }

beforeEach(() => {
  frames = []
  someRemoval.mockClear()
  Element.prototype.setPointerCapture = () => {}
  // Hand-driven frames: the scroller's loop is what we are exercising, and a
  // real clock would make when it runs a matter of how fast the box is.
  vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => {
    frames.push(cb)
    return frames.length
  })
  vi.stubGlobal('cancelAnimationFrame', () => {})
  stubHitTest()
})

afterEach(() => {
  cleanup()
  document.body.innerHTML = ''
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

// ── Tests ───────────────────────────────────────────────────────────

describe('useAgentChatsDrag without its chrome', () => {
  it('drags to a drop with no hairline and no ghost mounted', () => {
    const rowA = makeRow('a', 0)
    makeRow('b', 1)
    const { result, onDrop } = renderDrag()

    press(result, CHAT_A, rowA)
    move(10, 20)
    move(10, ROW_H + 2)
    release(10, ROW_H + 2)

    // The indicator and the ghost mount a render AFTER the drag begins, so a
    // drag that resolves a target in that window must not reach for either.
    expect(onDrop).toHaveBeenCalledTimes(1)
    expect(onDrop.mock.calls[0][1]).toMatchObject({ id: 'b', mode: 'before' })
  })

  it('stops the press from painting a text selection across the panel', () => {
    const rowA = makeRow('a', 0)
    const { result } = renderDrag()

    press(result, CHAT_A, rowA)
    // Armed on the PRESS, not on the drag: `selectstart` fires as the selection
    // begins, which is before the 5px threshold has promoted anything — arming
    // it at drag start is arming it after the only event it could cancel.
    const started = new Event('selectstart', { cancelable: true, bubbles: true })
    document.dispatchEvent(started)
    expect(started.defaultPrevented).toBe(true)

    release(10, 10)
    // …and it is dropped again the moment the pointer is up, whether or not the
    // press ever became a drag.
    const after = new Event('selectstart', { cancelable: true, bubbles: true })
    document.dispatchEvent(after)
    expect(after.defaultPrevented).toBe(false)
  })

  it('takes no pointer capture on a row that has left the DOM', () => {
    const rowA = makeRow('a', 0)
    makeRow('b', 1)
    const capture = vi.fn()
    Element.prototype.setPointerCapture = capture
    const { result } = renderDrag()

    press(result, CHAT_A, rowA)
    // A `deleted` frame lands between the press and the threshold. Capturing a
    // pointer on a detached node throws, and the drag has to survive it.
    rowA.remove()
    move(10, 20)

    expect(capture).not.toHaveBeenCalled()
    expect(result.current.dragging).toBe(true)
    release(10, 20)
  })

  it('carries the pressed element when the subject has no row of its own', () => {
    // The ghost clones what it can find; a subject whose row is not in the
    // window has nothing to clone, and the element under the hand is the
    // honest fallback.
    const stray = document.createElement('div')
    stubRect(stray, { top: 0, bottom: ROW_H, left: 0, right: 200, width: 200, height: ROW_H })
    document.body.appendChild(stray)
    const { result } = renderDrag()

    press(result, { kind: 'chat', id: 'ghost-only', parentId: '' }, stray)
    move(10, 20)

    expect(result.current.ghostRows?.count).toBe(1)
    expect(result.current.ghostRows?.nodes).toHaveLength(1)
    release(10, 20)
  })

  it('drags with no scroll container at all', () => {
    const rowA = makeRow('a', 0)
    makeRow('b', 1)
    const { result, onDrop } = renderDrag({ scroller: null })

    press(result, CHAT_A, rowA)
    move(10, 20)
    move(10, ROW_H + 2)
    // Nothing to scroll and nothing measured — the drag still resolves, because
    // the edge scroller is an affordance and not a dependency.
    expect(frames).toHaveLength(0)
    release(10, ROW_H + 2)
    expect(onDrop).toHaveBeenCalledTimes(1)
  })
})

describe('useAgentChatsDrag edge scrolling', () => {
  function withScroller() {
    const scroller = document.createElement('div')
    stubRect(scroller, {
      top: 0,
      bottom: 400,
      left: 0,
      right: 200,
      width: 200,
      height: 400,
    })
    document.body.appendChild(scroller)
    return scroller
  }

  it('scrolls while the pointer is held at an edge, and re-resolves what is under it', () => {
    const scroller = withScroller()
    const rowA = makeRow('a', 0)
    makeRow('b', 1)
    const { result, onDrop } = renderDrag({ scroller })

    press(result, CHAT_A, rowA, 10, 300)
    move(10, 290)
    // Inside the top band: the list runs toward what the hand is reaching for.
    move(10, 4)
    expect(frames.length).toBeGreaterThan(0)
    flushFrame()

    // The hairline is not mounted here, and the scroller telling it to stop
    // animating must not be the thing that throws.
    expect(result.current.dragging).toBe(true)

    // Back onto a row it may actually land on — y=4 is inside the row being
    // dragged, which refuses itself.
    move(10, ROW_H + 2)
    release(10, ROW_H + 2)
    expect(onDrop).toHaveBeenCalledTimes(1)
  })

  it('stops scrolling the moment the pointer leaves the panel on either side', () => {
    const scroller = withScroller()
    const rowA = makeRow('a', 0)
    makeRow('b', 1)
    const { result } = renderDrag({ scroller })

    press(result, CHAT_A, rowA, 10, 300)
    move(10, 290)
    move(10, 4)
    frames = []

    // Left of the panel and right of it are the same answer: the band is a
    // function of Y alone, so a row carried out sideways would otherwise keep
    // the list running under a hand that is trying to hold still.
    move(-30, 4)
    flushFrame()
    move(900, 4)
    flushFrame()

    expect(result.current.dragging).toBe(true)
    release(900, 4)
  })

  it('lets a spring fire even with nothing else mounted', () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    try {
      const rowA = makeRow('a', 0)
      const folded = makeRow('b', 1)
      // A folded row with something inside it: no `data-chat-expanded`, but
      // `data-chat-children` present.
      folded.removeAttribute('data-chat-expanded')
      folded.setAttribute('data-chat-children', '')
      const { result, onSpringOpen } = renderDrag()

      press(result, CHAT_A, rowA)
      move(10, 20)
      move(10, ROW_H + ROW_H / 2)
      act(() => vi.advanceTimersByTime(CHAT_SPRING_OPEN_MS + 10))

      expect(onSpringOpen).toHaveBeenCalledWith('b')
      release(10, ROW_H + ROW_H / 2)
    } finally {
      act(() => vi.runOnlyPendingTimers())
      vi.useRealTimers()
    }
  })
})

describe('useAgentChatsDrag over the editor pane', () => {
  /** Anything at or beyond this x is the editor pane; the panel is to its left. */
  const PANE_X = 400

  const overlay = () => document.querySelector<HTMLElement>('[data-pane-removal]')!
  /** The zone is DRAWN — up for the whole drag, in either of its two states. */
  const veilUp = () => !overlay().hidden
  /** A release right now would remove — the state the dwell unlocks. */
  const veilArmed = () => overlay().hasAttribute('data-armed')
  const veilTitle = () => overlay().querySelector('[data-pane-removal-title]')!.textContent

  /** The pane, the veil it carries, and a hit test that answers both. */
  function mountPane() {
    const pane = document.createElement('div')
    pane.setAttribute(PANE_DROP_ATTR, '')
    document.body.appendChild(pane)
    render(<EditorRemovalOverlay />, { container: pane })
    document.elementsFromPoint = ((x: number, y: number) => {
      if (x >= PANE_X) return [pane]
      for (const el of document.querySelectorAll<HTMLElement>('[data-chat-drop]')) {
        const r = el.getBoundingClientRect()
        if (r.height > 0 && y >= r.top && y < r.bottom) return [el]
      }
      return []
    }) as typeof document.elementsFromPoint
  }

  beforeEach(() => {
    // NOT `shouldAdvanceTime`: the dwell is measured in milliseconds, and a fake
    // clock that also creeps with the real one turns "one tick short of the
    // dwell" into a race with however long the test itself took to get there.
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('draws the zone for the WHOLE drag, and arms it only after the dwell', () => {
    const rowA = makeRow('a', 0)
    mountPane()
    const { result, onPaneRemove } = renderDrag()

    expect(veilUp()).toBe(false)

    press(result, CHAT_A, rowA)
    move(10, 20)
    // Up from the first frame, naming what would go — an affordance you have to
    // guess at is not one — but reading "Drop here", never "Release".
    expect(veilUp()).toBe(true)
    expect(veilArmed()).toBe(false)
    expect(veilTitle()).toBe('Drop here to remove First')

    move(PANE_X, 100)
    expect(veilArmed()).toBe(false)
    act(() => vi.advanceTimersByTime(PANE_ARM_MS))
    expect(veilArmed()).toBe(true)
    expect(veilTitle()).toBe('Release to remove First')

    release(PANE_X, 100)
    expect(onPaneRemove).toHaveBeenCalledWith([CHAT_A])
    expect(veilUp()).toBe(false)
  })

  it('removes nothing on a release that has not waited out the dwell', () => {
    const rowA = makeRow('a', 0)
    mountPane()
    const { result, onPaneRemove } = renderDrag()

    press(result, CHAT_A, rowA)
    move(10, 20)
    move(PANE_X, 100)
    act(() => vi.advanceTimersByTime(PANE_ARM_MS - 1))
    release(PANE_X, 100)

    // A long reorder crosses this pane on its way; a release mid-transit is a
    // drag that ended in the wrong place, not a delete.
    expect(onPaneRemove).not.toHaveBeenCalled()
  })

  it('falls back to available when the pointer leaves the pane again', () => {
    const rowA = makeRow('a', 0)
    makeRow('b', 1)
    mountPane()
    const { result, onPaneRemove, onDrop } = renderDrag()

    press(result, CHAT_A, rowA)
    move(10, 20)
    move(PANE_X, 100)
    act(() => vi.advanceTimersByTime(PANE_ARM_MS))
    expect(veilArmed()).toBe(true)

    move(10, ROW_H + 2)
    // Back to AVAILABLE, not away: the row is still in the air and the pane is
    // still where it would go.
    expect(veilUp()).toBe(true)
    expect(veilArmed()).toBe(false)

    release(10, ROW_H + 2)
    expect(onPaneRemove).not.toHaveBeenCalled()
    expect(onDrop).toHaveBeenCalledTimes(1)
  })

  it('re-entering the pane starts the dwell over', () => {
    const rowA = makeRow('a', 0)
    makeRow('b', 1)
    mountPane()
    const { result } = renderDrag()

    press(result, CHAT_A, rowA)
    move(10, 20)
    move(PANE_X, 100)
    act(() => vi.advanceTimersByTime(PANE_ARM_MS - 1))
    move(10, ROW_H + 2)
    move(PANE_X, 100)
    act(() => vi.advanceTimersByTime(PANE_ARM_MS - 1))
    expect(veilArmed()).toBe(false)

    act(() => vi.advanceTimersByTime(1))
    expect(veilArmed()).toBe(true)
    release(10, ROW_H + 2)
  })

  it('holding still inside the pane does not re-arm on every move', () => {
    const rowA = makeRow('a', 0)
    mountPane()
    const { result, removalText } = renderDrag()

    press(result, CHAT_A, rowA)
    move(10, 20)
    move(PANE_X, 100)
    act(() => vi.advanceTimersByTime(PANE_ARM_MS))
    const painted = removalText.mock.calls.length

    move(PANE_X, 120)
    move(PANE_X, 140)

    // The veil is a full-pane element with a backdrop filter behind it; a
    // repaint per pointermove is exactly what the dwell state machine exists to
    // avoid.
    expect(removalText.mock.calls.length).toBe(painted)
    release(PANE_X, 140)
  })

  it('never arms — and never veils — when there is nothing to remove', () => {
    const rowA = makeRow('a', 0)
    mountPane()
    const nothing: RemovalTextMock = vi.fn(() => null)
    const { result, onPaneRemove } = renderDrag({ removalText: nothing })

    press(result, CHAT_A, rowA)
    move(10, 20)
    expect(veilUp()).toBe(false)

    move(PANE_X, 100)
    act(() => vi.advanceTimersByTime(PANE_ARM_MS))
    // Better to offer no target than one that refuses on release: the rows this
    // drag is carrying have left the store while it was in the air.
    expect(veilUp()).toBe(false)
    release(PANE_X, 100)
    expect(onPaneRemove).not.toHaveBeenCalled()
  })

  it('takes the veil away when the drag ends anywhere else', () => {
    const rowA = makeRow('a', 0)
    mountPane()
    const { result } = renderDrag()

    press(result, CHAT_A, rowA)
    move(10, 20)
    expect(veilUp()).toBe(true)

    act(() => window.dispatchEvent(new MouseEvent('pointercancel', { bubbles: true })))
    expect(veilUp()).toBe(false)
  })

  it('takes the veil away when the panel unmounts mid-drag', () => {
    const rowA = makeRow('a', 0)
    mountPane()
    const { result, unmount } = renderDrag()

    press(result, CHAT_A, rowA)
    move(10, 20)
    move(PANE_X, 100)
    expect(veilUp()).toBe(true)

    // A workspace switch while a row is in the air would otherwise leave the
    // editor veiled with no drag left to end and clear it.
    unmount()
    expect(veilUp()).toBe(false)
  })
})
