import { act, fireEvent, render } from '@testing-library/react'
import { StrictMode, useEffect, useRef, useState } from 'react'
import { beforeEach, describe, expect, it } from 'vitest'
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
})
