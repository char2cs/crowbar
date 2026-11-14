import { useLayoutEffect } from 'react'
import { act, fireEvent, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  useTranscriptAnchor,
  type TranscriptAnchor,
  type TranscriptScrollPosition,
} from '@/features/agent/hooks/use-transcript-anchor'

/** jsdom has no layout: the scroller's metrics are test-held and its scrollTop
 *  clamps the way a browser's does, so a target past the ceiling is visible. */
let scrollHeight = 1000
let clientHeight = 400
let boxed = true
let anchor: TranscriptAnchor

interface HostProps {
  rev?: number
  measuring?: boolean
  loadingHistory?: boolean
  visible?: boolean
  arriving?: boolean
  initialPosition?: TranscriptScrollPosition | null
  onPositionChange?: (position: TranscriptScrollPosition) => void
}

// Mostly a reply streaming in: the case where growth eases.
function Host({ rev: _rev, measuring = false, arriving = true, ...options }: HostProps) {
  anchor = useTranscriptAnchor({ ...options, arriving })
  const { afterCommit } = anchor
  useLayoutEffect(() => afterCommit(measuring))
  return (
    <div
      data-testid="scroller"
      ref={(node) => {
        anchor.scrollRef.current = node
        if (!node || Object.hasOwn(node, 'scrollHeight')) return
        let top = 0
        Object.defineProperty(node, 'scrollTop', {
          configurable: true,
          get: () => (boxed ? top : 0),
          set: (v: number) => {
            if (boxed) top = Math.max(0, Math.min(v, Math.max(0, scrollHeight - clientHeight)))
          },
        })
        Object.defineProperty(node, 'scrollHeight', { get: () => (boxed ? scrollHeight : 0) })
        Object.defineProperty(node, 'clientHeight', { get: () => (boxed ? clientHeight : 0) })
      }}
      onScroll={() => anchor.onScroll()}
      {...anchor.intent}
    >
      <pre data-testid="inner" />
      <input data-testid="field" />
    </div>
  )
}

function mount(props: HostProps = {}) {
  let current = { ...props, rev: 0 }
  const view = render(<Host {...current} />)
  const scroller = view.getByTestId('scroller')
  const commit = (next: Partial<HostProps> = {}) => {
    current = { ...current, ...next, rev: current.rev + 1 }
    view.rerender(<Host {...current} />)
  }
  const grow = (height: number, next: Partial<HostProps> = {}) => {
    scrollHeight = height
    commit(next)
  }
  return { ...view, scroller, commit, grow }
}

/** A scroll nobody announced: the reader's gesture (or the browser's clamp). */
function readerScrollsTo(scroller: HTMLElement, top: number) {
  scroller.scrollTop = top
  fireEvent.scroll(scroller)
}

// A tool's payload: a box inside the transcript, scrolled to `top`.
function nestedBox(el: HTMLElement, top: number, overflowY = 'auto') {
  el.style.overflowY = overflowY
  Object.defineProperty(el, 'scrollHeight', { configurable: true, get: () => 500 })
  Object.defineProperty(el, 'clientHeight', { configurable: true, get: () => 100 })
  Object.defineProperty(el, 'scrollTop', { configurable: true, get: () => top })
  return el
}

const frames = (ms: number) => act(() => vi.advanceTimersByTime(ms))

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['requestAnimationFrame', 'cancelAnimationFrame', 'performance'] })
  scrollHeight = 1000
  clientHeight = 400
  boxed = true
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

describe('useTranscriptAnchor', () => {
  it('lands on the newest line at mount, in one write', () => {
    const { scroller } = mount()
    expect(scroller.scrollTop).toBe(600)
  })

  it('follows new rows with the eased follow while stuck to the bottom', () => {
    const { scroller, grow } = mount()
    grow(1300)
    frames(50)
    expect(scroller.scrollTop).toBeGreaterThan(600)
    expect(scroller.scrollTop).toBeLessThan(900)
    frames(1500)
    expect(scroller.scrollTop).toBe(900)
  })

  it('does not read its own follow writes as the reader scrolling away', () => {
    const { scroller, grow } = mount()
    grow(1300)
    frames(50)
    fireEvent.scroll(scroller)
    grow(1350)
    frames(1500)
    expect(scroller.scrollTop).toBe(950)
  })

  it('stops following when the reader scrolls up and resumes at the bottom', () => {
    const { scroller, grow } = mount()
    grow(1300)
    frames(50)
    readerScrollsTo(scroller, 200)
    grow(1600)
    frames(1500)
    expect(scroller.scrollTop).toBe(200)
    readerScrollsTo(scroller, 1200)
    grow(1800)
    frames(50)
    expect(scroller.scrollTop).toBeGreaterThan(1200)
    frames(1500)
    expect(scroller.scrollTop).toBe(1400)
  })

  it.each([
    [96, 900],
    [97, 503],
  ])("a scroll %i from the bottom isn't pulled down by a commit, and ends at %i", (gone, end) => {
    const { scroller, grow, commit } = mount()
    readerScrollsTo(scroller, 600 - gone)
    commit()
    expect(scroller.scrollTop).toBe(600 - gone)
    grow(1300)
    frames(1500)
    expect(scroller.scrollTop).toBe(end)
  })

  it('lets go on a wheel up, before the view has moved', () => {
    const { scroller, grow } = mount()
    grow(1300)
    frames(50)
    const at = scroller.scrollTop
    fireEvent.wheel(scroller, { deltaY: -100 })
    grow(1400)
    frames(1500)
    expect(scroller.scrollTop).toBe(at)
  })

  it("keeps reading while the wheel's own scroll climbs within a row of the bottom", () => {
    const { scroller, grow } = mount()
    fireEvent.wheel(scroller, { deltaY: -100 })
    readerScrollsTo(scroller, 600 - 30)
    grow(1300)
    frames(1500)
    expect(scroller.scrollTop).toBe(570)
    readerScrollsTo(scroller, 900 - 50) // back down within a row: rejoins the live end
    grow(1400)
    frames(1500)
    expect(scroller.scrollTop).toBe(1000)
  })

  it('lets go on a key that reads up, or a finger dragging the text down', () => {
    const keyed = mount()
    fireEvent.keyDown(keyed.scroller, { key: 'PageUp' })
    keyed.grow(1300)
    frames(1500)
    expect(keyed.scroller.scrollTop).toBe(600)
    keyed.unmount()
    scrollHeight = 1000
    const dragged = mount()
    fireEvent.touchStart(dragged.scroller, { touches: [{ clientY: 100 }] })
    fireEvent.touchMove(dragged.scroller, { touches: [{ clientY: 140 }] })
    dragged.grow(1300)
    frames(1500)
    expect(dragged.scroller.scrollTop).toBe(600)
  })

  it('keeps following when a wheel up scrolls a nested box that can still scroll up', () => {
    const { scroller, grow, getByTestId } = mount()
    const inner = nestedBox(getByTestId('inner'), 200)
    fireEvent.wheel(inner, { deltaY: -100 })
    grow(1300)
    frames(1500)
    expect(scroller.scrollTop).toBe(900)
  })

  it.each([
    ['at its top', 0, 'auto'],
    ['clipped, not a scroller', 200, 'hidden'],
  ])('lets go when the nested box is %s and the wheel chains on', (_, top, overflowY) => {
    const { scroller, grow, getByTestId } = mount()
    const inner = nestedBox(getByTestId('inner'), top, overflowY)
    fireEvent.wheel(inner, { deltaY: -100 })
    grow(1300)
    frames(1500)
    expect(scroller.scrollTop).toBe(600)
  })

  it('keeps following through a wheel down, a sideways wheel, a zoom and a jitter', () => {
    const { scroller, grow } = mount()
    fireEvent.wheel(scroller, { deltaY: 100 })
    fireEvent.wheel(scroller, { deltaX: -120, deltaY: -6 })
    fireEvent.wheel(scroller, { deltaY: -100, ctrlKey: true })
    fireEvent.wheel(scroller, { deltaY: -2 })
    grow(1300)
    frames(1500)
    expect(scroller.scrollTop).toBe(900)
  })

  it('takes keys in a field or a nested scroller as theirs, Shift+Space as reading up', () => {
    const { scroller, grow, getByTestId } = mount()
    fireEvent.keyDown(getByTestId('field'), { key: 'PageUp' })
    fireEvent.keyDown(nestedBox(getByTestId('inner'), 200), { key: 'ArrowUp' })
    grow(1300)
    frames(1500)
    expect(scroller.scrollTop).toBe(900)
    fireEvent.keyDown(scroller, { key: ' ', shiftKey: true })
    grow(1400)
    frames(1500)
    expect(scroller.scrollTop).toBe(900)
  })

  it('keeps the reader position when a row above the viewport grows', () => {
    const { scroller, grow } = mount()
    readerScrollsTo(scroller, 200)
    // The virtualizer moves the view by the row's growth and announces it.
    scrollHeight = 1060
    scroller.scrollTop = 260
    act(() => anchor.noteScrollWrite())
    fireEvent.scroll(scroller)
    grow(1060)
    frames(1500)
    expect(scroller.scrollTop).toBe(260)
  })

  it('does not read a virtualizer write as the reader leaving the bottom', () => {
    const { scroller, grow } = mount()
    grow(1300)
    frames(30)
    scroller.scrollTop = 500
    act(() => anchor.noteScrollWrite())
    fireEvent.scroll(scroller)
    grow(1350)
    frames(1500)
    expect(scroller.scrollTop).toBe(950)
  })

  it('lands instead of easing a gap bigger than the viewport', () => {
    const { scroller, grow } = mount()
    grow(1401)
    expect(scroller.scrollTop).toBe(1001)
  })

  it('eases a gap of exactly one viewport', () => {
    const { scroller, grow } = mount()
    grow(1400)
    expect(scroller.scrollTop).toBe(600)
  })

  it('judges a staircase of corrections by where the catch-up began', () => {
    const { scroller, grow } = mount()
    grow(1300)
    frames(50)
    // Each step is under a viewport from where the glide is; together they are not.
    grow(1450)
    expect(scroller.scrollTop).toBe(1050)
  })

  it('a commit with nothing to follow leaves no catch-up behind to ease from', () => {
    const { scroller, commit, grow } = mount()
    commit()
    commit({ measuring: true })
    grow(1150, { measuring: false })
    expect(scroller.scrollTop).toBe(750)
  })

  it('judges the jump from where the view is now when it moved up mid-catch-up', () => {
    const { scroller, grow } = mount()
    grow(1300)
    frames(16)
    scroller.scrollTop = 100
    act(() => anchor.noteScrollWrite())
    fireEvent.scroll(scroller)
    grow(1110)
    expect(scroller.scrollTop).toBe(710)
  })

  it('lands in one write when the viewport resizes under a stuck reader', () => {
    const { scroller } = mount()
    clientHeight = 250
    act(() => anchor.notifyReflow())
    expect(scroller.scrollTop).toBe(750)
  })

  it('reads a write the browser snapped to a device pixel as its own', () => {
    const { scroller, grow } = mount()
    grow(1300)
    frames(16)
    scroller.scrollTop += 0.5
    fireEvent.scroll(scroller)
    grow(1350)
    frames(1500)
    expect(scroller.scrollTop).toBe(950)
  })

  it('reads a write the scroller clamped as its own, not as the reader', () => {
    const { scroller, grow } = mount()
    readerScrollsTo(scroller, 500)
    boxed = false
    act(() => anchor.notifyReflow())
    // The pane grew while away: the held distance asks past the new ceiling.
    clientHeight = 600
    boxed = true
    act(() => anchor.notifyReflow())
    expect(scroller.scrollTop).toBe(400)
    fireEvent.scroll(scroller)
    grow(1300)
    frames(1500)
    expect(scroller.scrollTop).toBe(400)
  })

  it('lands every change while the history is still replaying', () => {
    const { scroller, grow } = mount({ loadingHistory: true })
    grow(1100)
    expect(scroller.scrollTop).toBe(700)
    grow(1200, { loadingHistory: false })
    frames(16)
    expect(scroller.scrollTop).toBeGreaterThan(700)
    expect(scroller.scrollTop).toBeLessThan(800)
  })

  it('lands growth in one write while nothing is arriving', () => {
    const { scroller, grow } = mount({ arriving: false })
    grow(1100)
    expect(scroller.scrollTop).toBe(700)
    grow(1200, { arriving: true })
    expect(scroller.scrollTop).toBe(700)
    frames(1500)
    expect(scroller.scrollTop).toBe(800)
  })

  it("lands the correction of a row's estimated height, then eases real growth", () => {
    const { scroller, grow } = mount({ measuring: true })
    grow(1150, { measuring: false })
    expect(scroller.scrollTop).toBe(750)
    grow(1250)
    frames(16)
    expect(scroller.scrollTop).toBeGreaterThan(750)
    expect(scroller.scrollTop).toBeLessThan(850)
  })

  it('keeps easing when a new row is corrected mid-glide', () => {
    const { scroller, grow } = mount()
    grow(1200, { measuring: true })
    frames(16)
    grow(1250, { measuring: false })
    expect(scroller.scrollTop).toBeLessThan(850)
    frames(1500)
    expect(scroller.scrollTop).toBe(850)
  })

  it('rejoins the live end when the reader sends a prompt', () => {
    const { scroller, grow } = mount()
    readerScrollsTo(scroller, 100)
    act(() => anchor.stick())
    grow(1300)
    frames(1500)
    expect(scroller.scrollTop).toBe(900)
  })

  it('restores the saved position when the chat view remounts', () => {
    const saved: TranscriptScrollPosition[] = []
    const first = mount({ onPositionChange: (p) => saved.push(p) })
    readerScrollsTo(first.scroller, 250)
    first.unmount()
    expect(saved).toEqual([{ stuck: false, distanceFromBottom: 750 }])
    scrollHeight = 1500
    const second = mount({ initialPosition: saved[0] })
    expect(second.scroller.scrollTop).toBe(750)
    second.grow(1700)
    frames(1500)
    expect(second.scroller.scrollTop).toBe(750)
  })

  // Rows at their estimates are too short for the distance: the clamp to 0 is not the reader's.
  it('holds a saved position until its rows measure into it', () => {
    const saved: TranscriptScrollPosition[] = []
    const held = { stuck: false, distanceFromBottom: 5000 }
    scrollHeight = 300
    mount({
      initialPosition: held,
      measuring: true,
      onPositionChange: saved.push.bind(saved),
    }).unmount()
    expect(saved).toEqual([held])
    const second = mount({ initialPosition: held, measuring: true })
    second.grow(8000, { measuring: false })
    expect(second.scroller.scrollTop).toBe(3000)
  })

  it('lands on the newest end, and follows, when the chat no longer holds the distance', () => {
    const held = { stuck: false, distanceFromBottom: 5000 }
    const { scroller, grow } = mount({ initialPosition: held, measuring: true })
    grow(3000, { measuring: false })
    expect(scroller.scrollTop).toBe(2600)
    grow(3400)
    frames(1500)
    expect(scroller.scrollTop).toBe(3000)
  })

  it('a saved stuck position lands at the bottom', () => {
    scrollHeight = 1500
    const { scroller } = mount({ initialPosition: { stuck: true, distanceFromBottom: 900 } })
    expect(scroller.scrollTop).toBe(1100)
  })

  it('lands at the bottom in one write when a display:none workspace comes back', () => {
    const { scroller, grow } = mount()
    boxed = false
    grow(1300)
    act(() => anchor.notifyReflow())
    boxed = true
    act(() => anchor.notifyReflow())
    expect(scroller.scrollTop).toBe(900)
  })

  it('lands when the list commits before the viewport reports its box is back', () => {
    const { scroller, grow } = mount()
    boxed = false
    act(() => anchor.notifyReflow())
    boxed = true
    grow(1300)
    expect(scroller.scrollTop).toBe(900)
  })

  it('holds a reader mid-history through a display:none round trip', () => {
    const { scroller } = mount()
    readerScrollsTo(scroller, 300)
    boxed = false
    act(() => anchor.notifyReflow())
    boxed = true
    scroller.scrollTop = 0
    act(() => anchor.notifyReflow())
    expect(scroller.scrollTop).toBe(300)
  })

  it('saves the position it last had a box for, not the zeros an eviction reads', () => {
    const saved: TranscriptScrollPosition[] = []
    const { scroller, unmount } = mount({ onPositionChange: (p) => saved.push(p) })
    readerScrollsTo(scroller, 300)
    boxed = false
    act(() => anchor.notifyReflow())
    unmount()
    expect(saved).toEqual([{ stuck: false, distanceFromBottom: 700 }])
  })

  it('lands instead of easing when a background tab comes to the front', () => {
    const { scroller, commit } = mount({ visible: false })
    scrollHeight = 1300
    commit({ visible: true })
    expect(scroller.scrollTop).toBe(900)
  })

  it('catches up on window focus after rAF froze mid-glide', () => {
    const { scroller, grow } = mount()
    const realRaf = globalThis.requestAnimationFrame
    let id = 0
    vi.stubGlobal('requestAnimationFrame', () => ++id)
    grow(1300)
    frames(1500)
    expect(scroller.scrollTop).toBe(600)
    vi.stubGlobal('requestAnimationFrame', realRaf)
    act(() => void window.dispatchEvent(new Event('focus')))
    frames(1500)
    expect(scroller.scrollTop).toBe(900)
  })
})
