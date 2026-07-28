import { act, fireEvent, render } from '@testing-library/react'
import { StrictMode, useEffect, useRef, useState } from 'react'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import {
  clearPreservedScroll,
  usePreservedScroll,
} from '@/features/editor/hooks/use-preserved-scroll'

/**
 * Minimal host for the hook: a scrollable div whose offset the hook is asked to
 * retain under `scrollKey`. Mirrors how the preview panes use it — the element
 * is destroyed and recreated on every remount, so the offset can only survive
 * outside the component.
 */
function ScrollHost({ scrollKey, ready = true }: { scrollKey: string | null; ready?: boolean }) {
  const ref = useRef<HTMLDivElement>(null)
  usePreservedScroll(ref, scrollKey, ready)
  return (
    <div ref={ref} data-testid="scroller" style={{ overflow: 'auto', height: 100 }}>
      <div style={{ height: 1000 }} />
    </div>
  )
}

describe('usePreservedScroll', () => {
  beforeEach(() => {
    clearPreservedScroll()
  })

  it('restores the last scroll offset after an unmount/remount cycle', () => {
    const first = render(<ScrollHost scrollKey="buffer-a" />)
    const scroller = first.getByTestId('scroller')

    scroller.scrollTop = 420
    fireEvent.scroll(scroller)

    first.unmount()

    const second = render(<ScrollHost scrollKey="buffer-a" />)
    expect(second.getByTestId('scroller').scrollTop).toBe(420)
  })

  it('keys the offset per buffer so a different buffer starts at the top', () => {
    const first = render(<ScrollHost scrollKey="buffer-a" />)
    const scroller = first.getByTestId('scroller')
    scroller.scrollTop = 420
    fireEvent.scroll(scroller)
    first.unmount()

    const second = render(<ScrollHost scrollKey="buffer-b" />)
    expect(second.getByTestId('scroller').scrollTop).toBe(0)
  })

  it('records the outgoing key, not the incoming one, when the key changes in place', () => {
    const view = render(<ScrollHost scrollKey="buffer-a" />)
    const scroller = view.getByTestId('scroller')
    scroller.scrollTop = 300
    fireEvent.scroll(scroller)

    // Same DOM node, different buffer: the new buffer must start at the top and
    // must not overwrite buffer-a's remembered offset.
    view.rerender(<ScrollHost scrollKey="buffer-b" />)
    expect(scroller.scrollTop).toBe(0)

    view.rerender(<ScrollHost scrollKey="buffer-a" />)
    expect(scroller.scrollTop).toBe(300)
  })

  it('waits for the content to be ready before restoring', () => {
    const first = render(<ScrollHost scrollKey="buffer-a" />)
    const scroller = first.getByTestId('scroller')
    scroller.scrollTop = 250
    fireEvent.scroll(scroller)
    first.unmount()

    // Content not painted yet — restoring now would be clamped to 0 by the
    // browser (empty scroll box), silently losing the offset.
    const second = render(<ScrollHost scrollKey="buffer-a" ready={false} />)
    expect(second.getByTestId('scroller').scrollTop).toBe(0)

    second.rerender(<ScrollHost scrollKey="buffer-a" ready={true} />)
    expect(second.getByTestId('scroller').scrollTop).toBe(250)
  })

  it('does not restore after the user scrolls back to the top', () => {
    const first = render(<ScrollHost scrollKey="buffer-a" />)
    const scroller = first.getByTestId('scroller')
    scroller.scrollTop = 500
    fireEvent.scroll(scroller)
    scroller.scrollTop = 0
    fireEvent.scroll(scroller)
    first.unmount()

    const second = render(<ScrollHost scrollKey="buffer-a" />)
    expect(second.getByTestId('scroller').scrollTop).toBe(0)
  })

  /**
   * The real preview parses its markdown in an effect, so the content lands one
   * commit AFTER mount — the same shape as this host.
   */
  function DeferredContentHost({ scrollKey }: { scrollKey: string }) {
    const ref = useRef<HTMLDivElement>(null)
    const [ready, setReady] = useState(false)
    useEffect(() => {
      setReady(true)
    }, [])
    usePreservedScroll(ref, scrollKey, ready)
    return (
      <div ref={ref} data-testid="scroller" style={{ overflow: 'auto', height: 100 }}>
        {ready && <div style={{ height: 1000 }} />}
      </div>
    )
  }

  // The app renders under StrictMode, which mounts every component twice: setup
  // → cleanup → setup. On the way back that first cleanup sees a BRAND NEW
  // element still sitting at 0 with no content yet, and must not record it —
  // doing so wipes the offset the restore is about to read, which is exactly how
  // the retained scroll was lost in the real app.
  it('survives StrictMode double-invoked effects when content arrives a commit late', () => {
    const first = render(
      <StrictMode>
        <DeferredContentHost scrollKey="buffer-a" />
      </StrictMode>,
    )
    const scroller = first.getByTestId('scroller')
    act(() => {
      scroller.scrollTop = 360
      fireEvent.scroll(scroller)
    })
    first.unmount()

    const second = render(
      <StrictMode>
        <DeferredContentHost scrollKey="buffer-a" />
      </StrictMode>,
    )
    expect(second.getByTestId('scroller').scrollTop).toBe(360)
  })

  it('is inert without a key', () => {
    const first = render(<ScrollHost scrollKey={null} />)
    const scroller = first.getByTestId('scroller')
    scroller.scrollTop = 180
    fireEvent.scroll(scroller)
    first.unmount()

    const second = render(<ScrollHost scrollKey={null} />)
    expect(second.getByTestId('scroller').scrollTop).toBe(0)
  })

  /**
   * Content that keeps growing after the first commit — a README's images and
   * embedded HTML sizing themselves, a diagram rendering — is the case jsdom
   * cannot show on its own, because it has no layout and therefore never clamps
   * anything. Both halves are modelled explicitly here: a scroll box that
   * truncates an out-of-range `scrollTop` like a real one, and a ResizeObserver
   * the test can fire to represent the content growing.
   */
  describe('content that is still growing', () => {
    let maxScroll = 0
    let observerCallbacks: Array<() => void> = []
    const RealResizeObserver = globalThis.ResizeObserver

    beforeEach(() => {
      maxScroll = 0
      observerCallbacks = []
      class ControllableResizeObserver {
        callback: () => void
        constructor(callback: () => void) {
          this.callback = callback
        }
        observe() {
          if (!observerCallbacks.includes(this.callback)) observerCallbacks.push(this.callback)
        }
        unobserve() {}
        disconnect() {
          observerCallbacks = observerCallbacks.filter((c) => c !== this.callback)
        }
      }
      Object.defineProperty(globalThis, 'ResizeObserver', {
        value: ControllableResizeObserver,
        configurable: true,
        writable: true,
      })
    })

    afterEach(() => {
      Object.defineProperty(globalThis, 'ResizeObserver', {
        value: RealResizeObserver,
        configurable: true,
        writable: true,
      })
    })

    /** Fires every live observer, as the browser would once a box changed. */
    const growContent = (newMax: number) => {
      maxScroll = newMax
      act(() => {
        for (const cb of [...observerCallbacks]) cb()
      })
    }

    function ClampingScroller({ scrollKey }: { scrollKey: string }) {
      const ref = useRef<HTMLDivElement>(null)
      usePreservedScroll(ref, scrollKey, true)
      return (
        <div
          data-testid="scroller"
          ref={(node) => {
            ref.current = node
            if (!node || Object.hasOwn(node, 'scrollTop')) return
            let value = 0
            Object.defineProperty(node, 'scrollTop', {
              configurable: true,
              get: () => value,
              set: (next: number) => {
                value = Math.max(0, Math.min(next, maxScroll))
              },
            })
          }}
        />
      )
    }

    // ALWAYS under StrictMode, like the app root, and with StrictMode as the
    // ROOT of the render — behind an intermediate component the double-invoke
    // had not flushed by the time the assertions ran, and this suite went green
    // against a hook that did nothing in the real app.
    //
    // The double-invoke is the whole point: setup → cleanup → setup means the
    // first cleanup tears down the growth watcher, and the second setup is past
    // the once-per-key restore guard. A watcher owned by that guard is dead on
    // arrival, and the offset stays truncated.
    const renderClamping = (scrollKey: string) =>
      render(
        <StrictMode>
          <ClampingScroller scrollKey={scrollKey} />
        </StrictMode>,
      )

    // Regression: restoring once, before images and embedded HTML have sized
    // themselves, lands partway down and stays there — the offset is silently
    // truncated to whatever fitted at that instant.
    it('re-applies the offset as the content grows tall enough to hold it', () => {
      maxScroll = 900
      const first = renderClamping('buffer-a')
      const scroller = first.getByTestId('scroller')
      scroller.scrollTop = 600
      fireEvent.scroll(scroller)
      first.unmount()

      // Remount with a document that has only partly rendered.
      maxScroll = 200
      const second = renderClamping('buffer-a')
      expect(second.getByTestId('scroller').scrollTop).toBe(200) // clamped

      growContent(900)
      expect(second.getByTestId('scroller').scrollTop).toBe(600)
    })

    // The clamped intermediate is not a reading position: recording it would
    // replace the real offset with a truncated one, and every later restore
    // would inherit the smaller value.
    it('does not record the clamped offset if the tab is switched again mid-growth', () => {
      maxScroll = 900
      const first = renderClamping('buffer-a')
      const scroller = first.getByTestId('scroller')
      scroller.scrollTop = 600
      fireEvent.scroll(scroller)
      first.unmount()

      maxScroll = 200
      const second = renderClamping('buffer-a')
      expect(second.getByTestId('scroller').scrollTop).toBe(200)
      second.unmount() // switched away again before the content finished

      maxScroll = 900
      const third = renderClamping('buffer-a')
      expect(third.getByTestId('scroller').scrollTop).toBe(600)
    })

    it('stops chasing the offset once the user scrolls', () => {
      maxScroll = 900
      const first = renderClamping('buffer-a')
      const scroller = first.getByTestId('scroller')
      scroller.scrollTop = 600
      fireEvent.scroll(scroller)
      first.unmount()

      maxScroll = 200
      const second = renderClamping('buffer-a')
      const node = second.getByTestId('scroller')
      expect(node.scrollTop).toBe(200)

      // The user takes over while the rest of the document is still loading.
      act(() => {
        node.scrollTop = 50
        fireEvent.scroll(node)
      })
      growContent(900)

      expect(node.scrollTop).toBe(50)
    })
  })
})
