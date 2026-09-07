import type { RefObject } from 'react'
import { describe, expect, it, beforeEach, afterEach, vi } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import {
  pickBySpecificity,
  pointInRect,
  type RegisteredConsumer,
  useTauriFileDrop,
} from '@/features/file-system/lib/tauri-file-drop'

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

function consumerFor(el: HTMLElement | null): RegisteredConsumer {
  return { containerRef: { current: el }, onDrop: vi.fn() }
}

// `resolveConsumer`'s two callers (the elementFromPoint hit-test and the
// rect fallback) both pre-filter to registrations with a live, distinct DOM
// node before ever calling this, so its own null-ref and identical-node
// guards are unreachable through the hook — exercised directly here instead.
describe('pickBySpecificity', () => {
  it('returns undefined for an empty candidate list', () => {
    expect(pickBySpecificity([])).toBeUndefined()
  })

  it('skips a candidate whose ref has already gone null, keeping the champion', () => {
    const champion = consumerFor(document.createElement('div'))
    const goneRef = consumerFor(null)

    expect(pickBySpecificity([champion, goneRef])).toBe(champion)
  })

  it('keeps the champion when a later candidate resolves to the identical DOM node', () => {
    const el = document.createElement('div')
    const champion = consumerFor(el)
    const duplicateRegistration = consumerFor(el)

    expect(pickBySpecificity([champion, duplicateRegistration])).toBe(champion)
  })

  it('picks the descendant regardless of registration order — ancestor-then-descendant', () => {
    const inner = document.createElement('div')
    const outer = document.createElement('div')
    outer.appendChild(inner)

    const innerConsumer = consumerFor(inner)
    const outerConsumer = consumerFor(outer)

    expect(pickBySpecificity([outerConsumer, innerConsumer])).toBe(innerConsumer)
  })

  it('picks the descendant regardless of registration order — descendant-then-ancestor', () => {
    const inner = document.createElement('div')
    const outer = document.createElement('div')
    outer.appendChild(inner)

    const innerConsumer = consumerFor(inner)
    const outerConsumer = consumerFor(outer)

    // The descendant registers (mounts) FIRST here, becoming the initial
    // "champion" the reduce starts from — exercising the other half of the
    // containment check (`candidateEl.contains(championEl)`) than the
    // ancestor-then-descendant ordering above.
    expect(pickBySpecificity([innerConsumer, outerConsumer])).toBe(innerConsumer)
  })

  it('keeps the champion over a larger, unrelated (non-nested) later candidate', () => {
    const small = document.createElement('div')
    small.getBoundingClientRect = () => rect(150, 150, 250, 250)
    const large = document.createElement('div')
    large.getBoundingClientRect = () => rect(0, 0, 400, 400)
    const smallConsumer = consumerFor(small)

    // small (mounted first) is the initial champion; large's area is NOT
    // smaller, so it must not displace it — the other half of the area
    // tiebreak than the smaller-candidate-wins case the cross-consumer
    // arbitration tests below exercise.
    expect(pickBySpecificity([smallConsumer, consumerFor(large)])).toBe(smallConsumer)
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
    capturedCallback!({
      payload: { type: 'drop', position: { x: 50, y: 50 }, paths: ['/a/b.png'] },
    })

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
    capturedCallback!({
      payload: { type: 'drop', position: { x: 50, y: 50 }, paths: ['/a/b.png'] },
    })

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

// Regression coverage for C2: every mounted `useTauriFileDrop` consumer
// shares ONE underlying Tauri subscription (see `ensureSharedSubscription`),
// which arbitrates a single drop event down to exactly ONE handler — never
// zero, never both. Each test below mounts multiple consumers against the
// SAME mocked `onDragDropEvent` and replays the one captured drop callback,
// exactly as the real (single, shared) broadcast subscription does.
describe('useTauriFileDrop — cross-consumer arbitration', () => {
  const originalElementFromPoint = document.elementFromPoint

  beforeEach(() => {
    onDragDropEvent.mockReset()
    stop.mockReset()
    isTauriMock.mockReset()
    isTauriMock.mockReturnValue(true)
    // jsdom has no elementFromPoint at all — tests that want the real
    // hit-testing path install their own stub and restore it below.
    // @ts-expect-error -- deleting a possibly-absent jsdom API for a clean slate
    delete document.elementFromPoint
  })

  afterEach(() => {
    if (originalElementFromPoint) document.elementFromPoint = originalElementFromPoint
    // @ts-expect-error -- see beforeEach: only restore if jsdom ever had one
    else delete document.elementFromPoint
  })

  function mockElement(r: DOMRect): HTMLElement {
    const el = document.createElement('div')
    el.getBoundingClientRect = () => r
    return el
  }

  async function mountTwo(
    outerOnDrop: (paths: string[]) => void,
    innerOnDrop: (paths: string[]) => void,
    outerEl: HTMLElement,
    innerEl: HTMLElement,
  ): Promise<DragDropCallback[]> {
    const callbacks: DragDropCallback[] = []
    onDragDropEvent.mockImplementation(async (cb: DragDropCallback) => {
      callbacks.push(cb)
      return stop
    })
    renderHook(() => useTauriFileDrop({ current: outerEl }, outerOnDrop))
    renderHook(() => useTauriFileDrop({ current: innerEl }, innerOnDrop))
    // One shared subscription for both consumers — see `ensureSharedSubscription`.
    await waitFor(() => expect(callbacks.length).toBe(1))
    return callbacks
  }

  it('reproduces the confirmed C2 bug: a drop on the composer pill (nested in the pane) only reaches the composer, not the pane', async () => {
    // Mirrors pane-container.tsx (containerRef, the whole pane) and
    // agent-composer.tsx (pillRef, nested inside it) — dropping ON the pill
    // must never also fire the pane's handleFileOpen.
    const paneEl = mockElement(rect(0, 0, 400, 400))
    const pillEl = mockElement(rect(20, 320, 380, 360))
    paneEl.appendChild(pillEl)

    const paneDrop = vi.fn()
    const pillDrop = vi.fn()
    const callbacks = await mountTwo(paneDrop, pillDrop, paneEl, pillEl)

    for (const cb of callbacks) {
      cb({ payload: { type: 'drop', position: { x: 100, y: 340 }, paths: ['/a/file.pdf'] } })
    }

    expect(pillDrop).toHaveBeenCalledWith(['/a/file.pdf'])
    expect(paneDrop).not.toHaveBeenCalled()
  })

  it('still opens on the pane when the drop lands outside the nested composer pill', async () => {
    const paneEl = mockElement(rect(0, 0, 400, 400))
    const pillEl = mockElement(rect(20, 320, 380, 360))
    paneEl.appendChild(pillEl)

    const paneDrop = vi.fn()
    const pillDrop = vi.fn()
    const callbacks = await mountTwo(paneDrop, pillDrop, paneEl, pillEl)

    for (const cb of callbacks) {
      cb({ payload: { type: 'drop', position: { x: 100, y: 50 }, paths: ['/a/file.pdf'] } })
    }

    expect(paneDrop).toHaveBeenCalledWith(['/a/file.pdf'])
    expect(pillDrop).not.toHaveBeenCalled()
  })

  it('dispatches to the smaller of two unrelated (non-nested) overlapping consumers, e.g. a portal-rendered modal over the pane', async () => {
    // attach-file-modal.tsx's dropzone renders through a Dialog portal — it is
    // NOT a DOM descendant of pane-container.tsx's containerRef, so this
    // exercises the area tiebreak rather than DOM containment.
    const paneEl = mockElement(rect(0, 0, 400, 400))
    const modalEl = mockElement(rect(150, 150, 250, 250)) // not appended under paneEl

    const paneDrop = vi.fn()
    const modalDrop = vi.fn()
    const callbacks = await mountTwo(paneDrop, modalDrop, paneEl, modalEl)

    for (const cb of callbacks) {
      cb({ payload: { type: 'drop', position: { x: 200, y: 200 }, paths: ['/b/img.png'] } })
    }

    expect(modalDrop).toHaveBeenCalledWith(['/b/img.png'])
    expect(paneDrop).not.toHaveBeenCalled()
  })

  it('prefers document.elementFromPoint over rect geometry when it is available', async () => {
    // Two consumers with the IDENTICAL rect (e.g. two terminal buffers kept
    // mounted in the same pane, one hidden via visibility:hidden) can never
    // be told apart by geometry alone — only real hit-testing (which skips
    // hidden elements) knows which one is actually on screen.
    const sameRect = rect(0, 0, 300, 300)
    const hiddenEl = mockElement(sameRect)
    const visibleEl = mockElement(sameRect)

    document.elementFromPoint = vi.fn(() => visibleEl)

    const hiddenDrop = vi.fn()
    const visibleDrop = vi.fn()
    const callbacks = await mountTwo(hiddenDrop, visibleDrop, hiddenEl, visibleEl)

    for (const cb of callbacks) {
      cb({ payload: { type: 'drop', position: { x: 150, y: 150 }, paths: ['/c/note.txt'] } })
    }

    expect(visibleDrop).toHaveBeenCalledWith(['/c/note.txt'])
    expect(hiddenDrop).not.toHaveBeenCalled()
  })

  it('dispatches to nobody when elementFromPoint resolves outside every registered consumer', async () => {
    const paneEl = mockElement(rect(0, 0, 400, 400))
    const otherEl = mockElement(rect(0, 0, 400, 400))
    const unrelatedEl = document.createElement('div') // hit-tested element belongs to neither

    document.elementFromPoint = vi.fn(() => unrelatedEl)

    const paneDrop = vi.fn()
    const otherDrop = vi.fn()
    const callbacks = await mountTwo(paneDrop, otherDrop, paneEl, otherEl)

    for (const cb of callbacks) {
      cb({ payload: { type: 'drop', position: { x: 150, y: 150 }, paths: ['/d/z.txt'] } })
    }

    expect(paneDrop).not.toHaveBeenCalled()
    expect(otherDrop).not.toHaveBeenCalled()
  })

  it('dispatches to nobody when elementFromPoint itself returns null (a point outside any rendered element)', async () => {
    const paneEl = mockElement(rect(0, 0, 400, 400))
    const otherEl = mockElement(rect(0, 0, 400, 400))

    document.elementFromPoint = vi.fn(() => null)

    const paneDrop = vi.fn()
    const otherDrop = vi.fn()
    const callbacks = await mountTwo(paneDrop, otherDrop, paneEl, otherEl)

    for (const cb of callbacks) {
      cb({ payload: { type: 'drop', position: { x: 150, y: 150 }, paths: ['/e/z.txt'] } })
    }

    expect(paneDrop).not.toHaveBeenCalled()
    expect(otherDrop).not.toHaveBeenCalled()
  })
})
