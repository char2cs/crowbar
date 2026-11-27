import { act, fireEvent, render } from '@testing-library/react'
import { StrictMode, useEffect, useRef, useState } from 'react'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { usePreservedScroll } from '@/features/editor/hooks/use-preserved-scroll'

function ScrollHost({ scrollKey, ready = true }: { scrollKey: string | null; ready?: boolean }) {
  const ref = useRef<HTMLDivElement>(null)
  usePreservedScroll(ref, scrollKey === null ? null : `${testRun}:${scrollKey}`, ready)
  return (
    <div ref={ref} data-testid="scroller" style={{ overflow: 'auto', height: 100 }}>
      <div style={{ height: 1000 }} />
    </div>
  )
}

let testRun = 0

describe('usePreservedScroll', () => {
  beforeEach(() => {
    testRun += 1
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

    const renderClamping = (scrollKey: string) =>
      render(
        <StrictMode>
          <ClampingScroller scrollKey={scrollKey} />
        </StrictMode>,
      )

    it('re-applies the offset as the content grows tall enough to hold it', () => {
      maxScroll = 900
      const first = renderClamping('buffer-a')
      const scroller = first.getByTestId('scroller')
      scroller.scrollTop = 600
      fireEvent.scroll(scroller)
      first.unmount()

      maxScroll = 200
      const second = renderClamping('buffer-a')
      expect(second.getByTestId('scroller').scrollTop).toBe(200)

      growContent(900)
      expect(second.getByTestId('scroller').scrollTop).toBe(600)
    })

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
      second.unmount()

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

      act(() => {
        node.scrollTop = 50
        fireEvent.scroll(node)
      })
      growContent(900)

      expect(node.scrollTop).toBe(50)
    })
  })
})
