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
  initialPosition?: TranscriptScrollPosition | null
  onPositionChange?: (position: TranscriptScrollPosition) => void
}

function Host({ rev: _rev, measuring = false, ...options }: HostProps) {
  anchor = useTranscriptAnchor(options)
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
    />
  )
}

function mount(props: HostProps = {}) {
  let current = { ...props, rev: 0 }
  const view = render(<Host {...current} />)
  const scroller = view.getByTestId('scroller')
  /** The list commits, as it does after every virtualizer re-measure. */
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

  it('a scroll within a row of the bottom keeps following', () => {
    const { scroller, grow } = mount()
    readerScrollsTo(scroller, 600 - 96)

    grow(1300)
    frames(1500)
    expect(scroller.scrollTop).toBe(900)
  })

  it('a scroll just past a row from the bottom stops following', () => {
    const { scroller, grow } = mount()
    readerScrollsTo(scroller, 600 - 97)

    grow(1300)
    frames(1500)
    expect(scroller.scrollTop).toBe(503)
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

  it('lands every change while the history is still replaying', () => {
    const { scroller, grow } = mount({ loadingHistory: true })
    grow(1100)
    expect(scroller.scrollTop).toBe(700)

    grow(1200, { loadingHistory: false })
    frames(16)
    expect(scroller.scrollTop).toBeGreaterThan(700)
    expect(scroller.scrollTop).toBeLessThan(800)
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
