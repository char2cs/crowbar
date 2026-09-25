import { describe, it, expect, vi } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { usePtySizeSync } from '@/features/terminal/hooks/use-pty-size-sync'

type SyncOptions = Parameters<typeof usePtySizeSync>[0]

// usePtySizeSync is the ONE owner of a view's PTY size. These pin what the old
// refitAndSyncPty helper pinned, now at the owner: the PTY is pushed whenever
// xterm's grid differs from what THIS connection was last told — even when a fit
// changes nothing — and never redundantly, and only by a visible view.

function makeTerminal(rows: number, cols: number) {
  let onResize: (() => void) | null = null
  return {
    rows,
    cols,
    onResize: vi.fn((cb: () => void) => {
      onResize = cb
      return { dispose: () => {} }
    }),
    fireResize(nextRows: number, nextCols: number) {
      this.rows = nextRows
      this.cols = nextCols
      onResize?.()
    },
  }
}

function makeConnection() {
  return { alive: true, resize: vi.fn() }
}

// jsdom gives every element a 0×0 box; the tests control it.
function makeContainer(width = 800, height = 600) {
  const el = document.createElement('div')
  el.getBoundingClientRect = () => ({ width, height }) as DOMRect
  return el
}

function render(props: {
  terminal: ReturnType<typeof makeTerminal>
  connection: ReturnType<typeof makeConnection> | null
  isVisible?: boolean
  fit?: () => void
}) {
  const fitAddon = { fit: props.fit ?? vi.fn() }
  const container = makeContainer()
  return renderHook(
    (p: { connection: ReturnType<typeof makeConnection> | null; isVisible: boolean }) =>
      usePtySizeSync({
        terminal: props.terminal as unknown as SyncOptions['terminal'],
        fitAddon: fitAddon as unknown as SyncOptions['fitAddon'],
        container,
        connection: p.connection as unknown as SyncOptions['connection'],
        isVisible: p.isVisible,
      }),
    { initialProps: { connection: props.connection, isVisible: props.isVisible ?? true } },
  )
}

describe('usePtySizeSync', () => {
  it("pushes xterm's size on attach even when the fit changes nothing (pane-split remount)", () => {
    const terminal = makeTerminal(28, 200)
    const connection = makeConnection()
    render({ terminal, connection })
    expect(connection.resize).toHaveBeenCalledExactlyOnceWith(28, 200)
  })

  it("pushes when xterm's grid changes, and never for an unchanged grid", () => {
    const terminal = makeTerminal(28, 200)
    const connection = makeConnection()
    render({ terminal, connection })
    connection.resize.mockClear()

    act(() => terminal.fireResize(28, 200))
    expect(connection.resize).not.toHaveBeenCalled()

    act(() => terminal.fireResize(40, 120))
    expect(connection.resize).toHaveBeenCalledExactlyOnceWith(40, 120)
  })

  it('re-pushes to a new connection (a re-attach) even at the same size', () => {
    const terminal = makeTerminal(28, 200)
    const first = makeConnection()
    const { rerender } = render({ terminal, connection: first })
    const second = makeConnection()
    rerender({ connection: second, isVisible: true })
    expect(second.resize).toHaveBeenCalledExactlyOnceWith(28, 200)
  })

  it('a hidden view never pushes; shown, it takes the PTY size back', () => {
    const terminal = makeTerminal(28, 200)
    const connection = makeConnection()
    const { rerender } = render({ terminal, connection, isVisible: false })
    act(() => terminal.fireResize(30, 90))
    expect(connection.resize).not.toHaveBeenCalled()

    rerender({ connection, isVisible: true })
    expect(connection.resize).toHaveBeenCalledExactlyOnceWith(30, 90)
  })

  it('fits xterm to the container before reconciling', () => {
    const terminal = makeTerminal(24, 80)
    const connection = makeConnection()
    const fit = vi.fn(() => {
      terminal.rows = 50
      terminal.cols = 160
    })
    render({ terminal, connection, fit })
    expect(fit).toHaveBeenCalled()
    expect(connection.resize).toHaveBeenCalledExactlyOnceWith(50, 160)
  })
})
