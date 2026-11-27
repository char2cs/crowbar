import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { stubLayout } from '@/__tests__/__mocks__/stub-layout'
import { ConsoleLog } from '@/features/console/components/console-log'
import type { ConsoleEntry } from '@/features/console/lib/entries'

function entries(n: number): ConsoleEntry[] {
  return Array.from({ length: n }, (_, i) => ({
    id: i + 1,
    kind: 'raw' as const,
    text: `line ${i + 1}`,
  }))
}

let restoreLayout: () => void
beforeEach(() => {
  restoreLayout = stubLayout()
})
afterEach(() => restoreLayout())

const log = () => document.querySelector('[data-slot="console-log"]') as HTMLElement
const bottom = () => log().scrollHeight - log().clientHeight
const props = { expandedId: null, onToggle: () => {}, open: true } as const

function userScrollTo(top: number) {
  fireEvent.wheel(log())
  log().scrollTop = top
  fireEvent.scroll(log())
}

describe('ConsoleLog', () => {
  it('mounts only a window of a large buffer', () => {
    render(<ConsoleLog entries={entries(5000)} {...props} />)
    const mounted = log().querySelectorAll('[data-index]').length
    expect(mounted).toBeGreaterThan(0)
    expect(mounted).toBeLessThan(100)
  })

  it('starts at, and follows, the newest line', () => {
    const { rerender } = render(<ConsoleLog entries={entries(50)} {...props} />)
    expect(log().scrollTop).toBeGreaterThan(0)
    rerender(<ConsoleLog entries={entries(400)} {...props} />)
    expect(log().scrollTop).toBeGreaterThanOrEqual(bottom())
    fireEvent.scroll(log())
    expect(screen.getByText('line 400')).toBeTruthy()
    expect(screen.queryByText('line 1')).toBeNull()
  })

  it('stops following once the user scrolls up, and resumes at the bottom', () => {
    const { rerender } = render(<ConsoleLog entries={entries(50)} {...props} />)
    userScrollTo(100)
    rerender(<ConsoleLog entries={entries(60)} {...props} />)
    expect(log().scrollTop).toBe(100)

    userScrollTo(bottom())
    rerender(<ConsoleLog entries={entries(70)} {...props} />)
    expect(log().scrollTop).toBeGreaterThanOrEqual(bottom())
  })

  it('is not stopped by a scroll the user did not make', () => {
    const { rerender } = render(<ConsoleLog entries={entries(50)} {...props} />)
    log().scrollTop = 100
    fireEvent.scroll(log())
    rerender(<ConsoleLog entries={entries(60)} {...props} />)
    expect(log().scrollTop).toBeGreaterThanOrEqual(bottom())
  })

  it('stops counting scrolls as the users a moment after the last input', () => {
    vi.useFakeTimers()
    try {
      const { rerender } = render(<ConsoleLog entries={entries(50)} {...props} />)
      fireEvent.wheel(log())
      vi.advanceTimersByTime(300)
      log().scrollTop = 100
      fireEvent.scroll(log())
      rerender(<ConsoleLog entries={entries(60)} {...props} />)
      expect(log().scrollTop).toBeGreaterThanOrEqual(bottom())
    } finally {
      vi.useRealTimers()
    }
  })

  it('returns to the newest line when the console is shown again', () => {
    const { rerender } = render(<ConsoleLog entries={entries(50)} {...props} open={false} />)
    userScrollTo(100)
    rerender(<ConsoleLog entries={entries(50)} {...props} open={true} />)
    expect(log().scrollTop).toBeGreaterThanOrEqual(bottom())
  })

  it('takes focus when shown, so Escape reaches the panel', () => {
    render(<ConsoleLog entries={entries(2)} {...props} />)
    expect(document.activeElement).toBe(log())
  })

  it('keeps following the newest line when the viewport is resized', () => {
    const observers: ((entries: unknown[]) => void)[] = []
    vi.stubGlobal(
      'ResizeObserver',
      class {
        constructor(cb: (entries: unknown[]) => void) {
          observers.push(cb)
        }
        observe() {}
        unobserve() {}
        disconnect() {}
      },
    )
    try {
      render(<ConsoleLog entries={entries(200)} {...props} />)
      log().scrollTop = 0
      observers.forEach((cb) => cb([]))
      expect(log().scrollTop).toBeGreaterThan(0)
    } finally {
      vi.unstubAllGlobals()
    }
  })
})
