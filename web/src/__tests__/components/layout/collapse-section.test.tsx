/**
 * The collapse animation, on the one box each section owns.
 *
 * The failure this file exists for is silent: leave the inline `height` on the
 * box after an expand and the section looks perfect until a row is added to it,
 * at which point it can never grow again. Nothing about that reads as an
 * animation bug when you hit it.
 *
 * jsdom runs no transitions and reports no layout, so both are supplied: a
 * `scrollHeight` the box can measure, and the `transitionend` the browser would
 * have fired. Without the height stub the component takes its own "nothing
 * measurable to tween" path, which is what keeps every other test in the suite
 * synchronous.
 */
import { describe, it, expect, afterEach, vi } from 'vitest'
import { render, screen, act } from '@testing-library/react'
import { CollapseSection } from '@/components/layout/collapse-section'

const SECTION = '[data-collapse-section]'
const box = () => document.querySelector<HTMLElement>(SECTION)

/** Give every element a measurable height, the way a real layout would. */
function stubLayout(height = 120) {
  Object.defineProperty(HTMLElement.prototype, 'scrollHeight', {
    configurable: true,
    get: () => height,
  })
}

/**
 * The `transitionend` the browser would fire at the end of the tween.
 *
 * jsdom has no `TransitionEvent` constructor, so the one field the listener
 * reads is set on a plain Event instead.
 */
function endTransition(el: HTMLElement) {
  const event = new Event('transitionend', { bubbles: true })
  Object.defineProperty(event, 'propertyName', { value: 'height' })
  act(() => {
    el.dispatchEvent(event)
  })
}

function reduceMotion(on: boolean) {
  const prior = window.matchMedia
  window.matchMedia = ((query: string) => ({
    matches: on && query.includes('prefers-reduced-motion'),
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  })) as typeof window.matchMedia
  return () => {
    window.matchMedia = prior
  }
}

afterEach(() => {
  vi.useRealTimers()
  Reflect.deleteProperty(HTMLElement.prototype, 'scrollHeight')
})

describe('what the box renders', () => {
  it('is absent entirely while closed, so a folded section leaves no group behind', () => {
    render(
      <CollapseSection open={false} role="group">
        <div>child</div>
      </CollapseSection>,
    )
    expect(box()).toBeNull()
    expect(screen.queryByText('child')).toBeNull()
  })

  it('carries the role it was given, so the tree still announces depth', () => {
    render(
      <CollapseSection open role="group">
        <div>child</div>
      </CollapseSection>,
    )
    expect(screen.getByRole('group')).toBeInTheDocument()
  })
})

describe('the expand hands the box back to the layout', () => {
  it('clears the inline height once the tween finishes — the silent failure', () => {
    stubLayout()
    const { rerender } = render(
      <CollapseSection open={false} role="group">
        <div>child</div>
      </CollapseSection>,
    )

    rerender(
      <CollapseSection open role="group">
        <div>child</div>
      </CollapseSection>,
    )
    // Mid-tween: the box is pinned to its measured height and clipped.
    expect(box()!.style.height).toBe('120px')
    expect(box()!.style.overflow).toBe('hidden')

    endTransition(box()!)

    expect(box()!.style.height).toBe('')
    expect(box()!.style.overflow).toBe('')
    expect(box()!.style.transition).toBe('')
    expect(box()!.style.opacity).toBe('')
  })

  it('clears it on the timer too, when transitionend never arrives', () => {
    vi.useFakeTimers()
    stubLayout()
    const { rerender } = render(<CollapseSection open={false}>child</CollapseSection>)
    rerender(<CollapseSection open>child</CollapseSection>)
    expect(box()!.style.height).toBe('120px')

    act(() => {
      vi.advanceTimersByTime(200)
    })

    expect(box()!.style.height).toBe('')
  })

  it('animates height AND opacity, at the one measured curve', () => {
    stubLayout()
    const { rerender } = render(<CollapseSection open={false}>child</CollapseSection>)
    rerender(<CollapseSection open>child</CollapseSection>)

    const transition = box()!.style.transition
    expect(transition).toContain('height 120ms cubic-bezier(0.42, 0, 0.58, 1)')
    expect(transition).toContain('opacity 120ms cubic-bezier(0.42, 0, 0.58, 1)')
  })
})

describe('the close keeps its children until it has closed over them', () => {
  it('holds them mounted through the tween, then drops them', () => {
    stubLayout()
    const { rerender } = render(
      <CollapseSection open>
        <div>child</div>
      </CollapseSection>,
    )

    rerender(
      <CollapseSection open={false}>
        <div>child</div>
      </CollapseSection>,
    )
    // Still there — there has to be something for the box to close over.
    expect(screen.getByText('child')).toBeInTheDocument()
    expect(box()!.style.height).toBe('0px')

    endTransition(box()!)

    expect(screen.queryByText('child')).toBeNull()
  })
})

describe('prefers-reduced-motion', () => {
  it('runs no transition at all, in either direction', () => {
    const restore = reduceMotion(true)
    stubLayout()
    try {
      const { rerender } = render(
        <CollapseSection open={false}>
          <div>child</div>
        </CollapseSection>,
      )

      rerender(
        <CollapseSection open>
          <div>child</div>
        </CollapseSection>,
      )
      expect(box()!.style.transition).toBe('')
      expect(box()!.style.height).toBe('')

      rerender(
        <CollapseSection open={false}>
          <div>child</div>
        </CollapseSection>,
      )
      // Gone immediately: no tween to wait out.
      expect(screen.queryByText('child')).toBeNull()
    } finally {
      restore()
    }
  })
})
