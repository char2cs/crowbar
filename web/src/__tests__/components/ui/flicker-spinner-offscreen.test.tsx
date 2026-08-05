import { render } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'

/**
 * A spinner nobody can see still drives the compositor — the sidebar's panels
 * are an off-viewport scroll-snap carousel, and rows scroll out of view — so
 * the animation parks itself while off-screen.
 *
 * The gate must FAIL OPEN. A spinner frozen by a wrong guess reads as "hung",
 * which costs far more than the frames it saves.
 */

type Observed = { target: Element; callback: IntersectionObserverCallback }

const installObserver = () => {
  const observed: Observed[] = []
  const disconnected: IntersectionObserverCallback[] = []
  class FakeObserver {
    constructor(private callback: IntersectionObserverCallback) {}
    observe(target: Element) {
      observed.push({ target, callback: this.callback })
    }
    disconnect() {
      disconnected.push(this.callback)
    }
    unobserve() {}
    takeRecords() {
      return []
    }
  }
  vi.stubGlobal('IntersectionObserver', FakeObserver)
  return {
    observed,
    disconnected,
    report: (isIntersecting: boolean) => {
      for (const { target, callback } of observed) {
        callback(
          [{ target, isIntersecting } as IntersectionObserverEntry],
          {} as IntersectionObserver,
        )
      }
    },
  }
}

const stripOf = (container: HTMLElement) =>
  container.querySelector<HTMLElement>('[style*="flicker-strip"]')!

afterEach(() => vi.unstubAllGlobals())

describe('FlickerSpinner off-screen gating', () => {
  it('runs until told otherwise', () => {
    const observer = installObserver()
    const { container } = render(<FlickerSpinner />)

    // Nothing observed yet — the play state must not be paused.
    expect(stripOf(container).style.animationPlayState).not.toBe('paused')
    expect(observer.observed).toHaveLength(1)
  })

  it('observes the spinner box, not the clipped strip', () => {
    // The strip is N frame-widths wide and mostly clipped away; intersection
    // has to be judged on the box the user actually sees.
    const observer = installObserver()
    const { getByRole } = render(<FlickerSpinner />)
    expect(observer.observed[0].target).toBe(getByRole('status'))
  })

  it('pauses off-screen and resumes on the way back', () => {
    const observer = installObserver()
    const { container } = render(<FlickerSpinner />)

    observer.report(false)
    expect(stripOf(container).style.animationPlayState).toBe('paused')

    observer.report(true)
    expect(stripOf(container).style.animationPlayState).toBe('running')
  })

  it('disconnects on unmount', () => {
    const observer = installObserver()
    const { unmount } = render(<FlickerSpinner />)
    unmount()
    expect(observer.disconnected).toHaveLength(1)
  })

  it('renders normally where IntersectionObserver is unavailable', () => {
    vi.stubGlobal('IntersectionObserver', undefined)
    const { container, getByRole } = render(<FlickerSpinner />)
    expect(getByRole('status', { name: 'Loading' })).toBeInTheDocument()
    expect(stripOf(container).style.animationPlayState).not.toBe('paused')
  })
})
