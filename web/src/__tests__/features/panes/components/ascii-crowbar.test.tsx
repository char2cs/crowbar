import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act, render } from '@testing-library/react'
import AsciiCrowbar from '@/features/panes/components/ascii-crowbar'
import { INTRO_MS } from '@/features/panes/components/ascii-crowbar-renderer'

/**
 * The backdrop tumbles for a brief intro after mount and then settles on a
 * static frame with NOTHING scheduled — an empty pane is idle, and idle costs
 * nothing. During the intro it pauses only when invisible (offscreen, or the
 * tab hidden), never on window focus.
 *
 * rAF is stubbed and driven by hand — no timers, no real waiting.
 *
 * Rendered onto a `<canvas>` now (perf fix — see ascii-crowbar.tsx's own doc
 * comment): there is no `pre.textContent` to read any more, so every test that
 * used to inspect rendered DOM text instead inspects the SAME `ctx.fillText`
 * calls the component makes, via a shared instrumented `getContext` stub —
 * `rowsOf()` collects exactly one frame's worth (everything fillText'd since
 * the last `clearRect`, which the component calls once per frame before
 * re-drawing the rows).
 */

let pending: Map<number, FrameRequestCallback>
let nextRafId: number
let fillTextCalls: string[]
let lastFont: string
let clearRectCount: number

function flushFrame(t: number) {
  const due = [...pending.entries()]
  pending.clear()
  act(() => {
    for (const [, cb] of due) cb(t)
  })
}

/** Everything fillText'd since the last clearRect — one frame's rows, in order. */
function rowsOf(): string[] {
  return fillTextCalls
}

beforeEach(() => {
  pending = new Map()
  nextRafId = 1
  vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => {
    const id = nextRafId++
    pending.set(id, cb)
    return id
  })
  vi.stubGlobal('cancelAnimationFrame', (id: number) => {
    pending.delete(id)
  })
  // jsdom reports the document as never focused. The loop must not care — but
  // stub it so a test that regresses to reading it fails loudly rather than
  // passing for the wrong reason.
  vi.spyOn(document, 'hasFocus').mockReturnValue(false)

  fillTextCalls = []
  lastFont = ''
  clearRectCount = 0
  const mockCtx = {
    get font() {
      return lastFont
    },
    set font(v: string) {
      lastFont = v
    },
    fillStyle: '',
    textBaseline: 'alphabetic',
    measureText: (text: string) => ({ width: text.length * 8 }),
    fillText: (text: string) => {
      fillTextCalls.push(text)
    },
    clearRect: () => {
      fillTextCalls = []
      clearRectCount++
    },
    fillRect: () => {},
    setTransform: () => {},
  }
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(
    mockCtx as unknown as CanvasRenderingContext2D,
  )
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('AsciiCrowbar — idle costs nothing', () => {
  it('settles after the intro and leaves no frame or timer scheduled', () => {
    // rAF stays the hand-driven stub; only real timers are faked.
    vi.useFakeTimers({ toFake: ['setTimeout', 'setInterval', 'setImmediate'] })
    try {
      render(<AsciiCrowbar width={40} height={20} />)
      expect(pending.size).toBe(1)
      for (let t = 16; t <= INTRO_MS + 64; t += 16) flushFrame(t)
      expect(pending.size).toBe(0)
      expect(vi.getTimerCount()).toBe(0)
      // The settled frame is on the canvas.
      expect(rowsOf()).toHaveLength(20)

      // Nothing re-arms it: not focus, not visibility.
      act(() => {
        window.dispatchEvent(new Event('focus'))
        document.dispatchEvent(new Event('visibilitychange'))
      })
      expect(pending.size).toBe(0)
    } finally {
      vi.useRealTimers()
    }
  })
})

describe('AsciiCrowbar — the intro is gated on visibility, never on focus', () => {
  it('runs while the tab is visible', () => {
    render(<AsciiCrowbar />)
    expect(pending.size).toBe(1)
    flushFrame(16)
    // The loop re-arms itself, so exactly one frame stays queued.
    expect(pending.size).toBe(1)
  })

  it('starts even when it mounts into a window that is not the key window', () => {
    vi.mocked(document.hasFocus).mockReturnValue(false)
    render(<AsciiCrowbar />)
    expect(pending.size).toBe(1)
  })

  it('keeps tumbling when the window loses focus while the tab is still visible', () => {
    render(<AsciiCrowbar />)
    expect(document.visibilityState).not.toBe('hidden')
    expect(pending.size).toBe(1)

    act(() => {
      window.dispatchEvent(new Event('blur'))
    })

    // Clicking another app must not freeze the backdrop — this is the defect.
    expect(pending.size).toBe(1)
    flushFrame(16)
    expect(pending.size).toBe(1)
  })

  it('stops while the tab itself is hidden, and focus does not un-gate it', () => {
    const spy = vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden')
    try {
      render(<AsciiCrowbar />)
      act(() => {
        document.dispatchEvent(new Event('visibilitychange'))
      })
      expect(pending.size).toBe(0)

      act(() => {
        window.dispatchEvent(new Event('focus'))
      })

      // Nothing listens to focus any more; a hidden tab stays stopped.
      expect(pending.size).toBe(0)
    } finally {
      spy.mockRestore()
    }
  })

  it('schedules nothing after unmount', () => {
    const { unmount } = render(<AsciiCrowbar />)
    expect(pending.size).toBe(1)
    unmount()
    expect(pending.size).toBe(0)

    act(() => {
      window.dispatchEvent(new Event('focus'))
      window.dispatchEvent(new Event('blur'))
      document.dispatchEvent(new Event('visibilitychange'))
    })

    // A stale listener would re-arm the loop for an unmounted component.
    expect(pending.size).toBe(0)
  })

  it('draws a single static frame under prefers-reduced-motion and never schedules one', () => {
    const original = window.matchMedia
    window.matchMedia = ((query: string) => ({
      matches: query.includes('prefers-reduced-motion'),
      media: query,
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    })) as typeof window.matchMedia
    try {
      render(<AsciiCrowbar width={40} height={20} />)
      expect(rowsOf()).toHaveLength(20)
      expect(pending.size).toBe(0)
    } finally {
      window.matchMedia = original
    }
  })
})

// The art itself is a deliberate design choice: gating must not move the grid,
// the frame rate or the point count. These pin the rendering so a future
// "optimisation" of the idle path can't quietly trade visual quality.
describe('AsciiCrowbar — the rendering is untouched by the idle gate', () => {
  it('renders exactly the grid it was asked for', () => {
    render(<AsciiCrowbar width={76} height={34} />)
    const rows = rowsOf()
    expect(rows).toHaveLength(34)
    expect(rows[0]).toHaveLength(76)
  })

  it('renders a large grid at full size — no cell cap', () => {
    render(<AsciiCrowbar width={500} height={280} />)
    const rows = rowsOf()
    expect(rows).toHaveLength(280)
    expect(rows[0]).toHaveLength(500)
  })

  it('keeps the requested glyph size', () => {
    render(<AsciiCrowbar width={40} height={20} fontSize={9} />)
    expect(lastFont).toContain('9px')
  })

  it('still lets ~30 frames per second of wall clock through the interval gate', () => {
    render(<AsciiCrowbar width={40} height={20} />)

    // Count real writes (clearRect, once per rendered frame) rather than
    // visible differences: a slow tumble on a small grid can produce two
    // identical frames in a row.
    clearRectCount = 0

    // 1ms steps so the measurement resolves the gate itself rather than the
    // aliasing of a coarse tick against it.
    for (let t = 1; t <= 1000; t++) flushFrame(t)

    expect(clearRectCount).toBeGreaterThanOrEqual(28)
    expect(clearRectCount).toBeLessThanOrEqual(31)
  })
})

/**
 * Re-gridding is the expensive half of this component: two typed-array
 * reallocations, a `canvas.width` write that drops the backing store, and a
 * full 15000-point re-render. The ResizeObserver used to run all of it
 * SYNCHRONOUSLY, inside the observer callback — so a pane sash drag, which
 * rewrites the pane's flex-basis on every raw pointermove past a 5.4px glyph
 * cell, paid it on nearly every one. Live-measured in the dev app it was
 * 216ms of callback time across a ~2s drag, the largest single cost in it, and
 * the `canvas.style` writes inside the callback re-dirtied layout on top
 * ("ResizeObserver loop completed with undelivered notifications").
 *
 * `measure()` is observed through its FIRST read — `wrap.clientWidth` — rather
 * than through the canvas, because jsdom reports every box as 0x0, so the
 * grid-unchanged guard inside `measure` would swallow any canvas-side signal.
 */
describe('AsciiCrowbar — re-gridding is coalesced, and paused for a sash drag', () => {
  let roCallbacks: ResizeObserverCallback[]
  let widthReads: number

  const fireResize = () => {
    for (const cb of roCallbacks) cb([], {} as ResizeObserver)
  }

  beforeEach(() => {
    roCallbacks = []
    widthReads = 0
    vi.stubGlobal(
      'ResizeObserver',
      class {
        constructor(cb: ResizeObserverCallback) {
          roCallbacks.push(cb)
        }
        observe() {}
        unobserve() {}
        disconnect() {}
      },
    )
    vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockImplementation(() => {
      widthReads++
      return 0
    })
  })

  afterEach(() => {
    document.documentElement.removeAttribute('data-pane-resizing')
  })

  it('never measures inside the observer callback — it schedules a frame', () => {
    render(<AsciiCrowbar />)
    const readsAfterMount = widthReads

    fireResize()
    expect(widthReads).toBe(readsAfterMount)

    flushFrame(16)
    expect(widthReads).toBeGreaterThan(readsAfterMount)
  })

  it('coalesces a burst of observations into a single re-grid', () => {
    render(<AsciiCrowbar />)
    const readsAfterMount = widthReads

    // A drag's worth of observations inside one frame.
    for (let i = 0; i < 20; i++) fireResize()
    flushFrame(16)

    expect(widthReads).toBe(readsAfterMount + 1)
  })

  it('skips the re-grid for the span of a pane sash drag, then re-grids when it ends', () => {
    render(<AsciiCrowbar />)
    document.documentElement.setAttribute('data-pane-resizing', '1')
    const readsAtDragStart = widthReads

    for (let frame = 1; frame <= 10; frame++) {
      fireResize()
      flushFrame(frame * 16)
    }
    expect(widthReads).toBe(readsAtDragStart)

    document.documentElement.removeAttribute('data-pane-resizing')
    act(() => {
      window.dispatchEvent(new CustomEvent('pane-resize-end'))
    })
    flushFrame(200)
    expect(widthReads).toBe(readsAtDragStart + 1)
  })
})
