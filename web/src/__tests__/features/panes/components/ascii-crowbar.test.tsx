import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act, render } from '@testing-library/react'
import AsciiCrowbar from '@/features/panes/components/ascii-crowbar'

/**
 * The tumbling backdrop pauses ONLY when it is invisible — offscreen, or the tab
 * hidden. It must keep tumbling while the app window merely isn't the key
 * window: a background desktop window is still on screen, and a backdrop that
 * freezes the instant you click another app is a visible defect. These tests pin
 * that, so a future CPU "optimisation" can't reintroduce a focus/blur gate.
 *
 * rAF is stubbed and driven by hand — no timers, no real waiting.
 */

let pending: Map<number, FrameRequestCallback>
let nextRafId: number

function flushFrame(t: number) {
  const due = [...pending.entries()]
  pending.clear()
  act(() => {
    for (const [, cb] of due) cb(t)
  })
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
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('AsciiCrowbar — the frame loop is gated on visibility, never on focus', () => {
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
      const { container } = render(<AsciiCrowbar width={40} height={20} />)
      const pre = container.querySelector('pre')!
      expect(pre.textContent?.split('\n')).toHaveLength(20)
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
    const { container } = render(<AsciiCrowbar width={76} height={34} />)
    const rows = container.querySelector('pre')!.textContent!.split('\n')
    expect(rows).toHaveLength(34)
    expect(rows[0]).toHaveLength(76)
  })

  it('renders a large grid at full size — no cell cap', () => {
    const { container } = render(<AsciiCrowbar width={500} height={280} />)
    const rows = container.querySelector('pre')!.textContent!.split('\n')
    expect(rows).toHaveLength(280)
    expect(rows[0]).toHaveLength(500)
  })

  it('keeps the requested glyph size', () => {
    const { container } = render(<AsciiCrowbar width={40} height={20} fontSize={9} />)
    expect(container.querySelector('pre')!.style.fontSize).toBe('9px')
  })

  it('still lets ~30 frames per second of wall clock through the interval gate', () => {
    const { container } = render(<AsciiCrowbar width={40} height={20} />)
    const pre = container.querySelector('pre')!

    // Count real writes rather than visible differences: a slow tumble on a
    // small grid can produce two identical frames in a row.
    let writes = 0
    let held = pre.textContent
    Object.defineProperty(pre, 'textContent', {
      configurable: true,
      get: () => held,
      set: (v: string) => {
        held = v
        writes++
      },
    })

    // 1ms steps so the measurement resolves the gate itself rather than the
    // aliasing of a coarse tick against it.
    for (let t = 1; t <= 1000; t++) flushFrame(t)

    expect(writes).toBeGreaterThanOrEqual(28)
    expect(writes).toBeLessThanOrEqual(31)
  })
})
