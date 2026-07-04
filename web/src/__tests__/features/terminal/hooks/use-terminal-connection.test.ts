import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'

// vi.hoisted runs before the vi.mock factories below, so the bridge spies and
// the captured listen callback exist when the mock module is constructed.
const bridge = vi.hoisted(() => {
  let listenCb: ((frame: { data: string; snapshot: boolean }) => void) | null = null
  return {
    terminalWrite: vi.fn(async () => {}),
    terminalResize: vi.fn(async () => {}),
    terminalResync: vi.fn(async () => {}),
    terminalClose: vi.fn(async () => {}),
    terminalListen: vi.fn(
      (_id: string, onFrame: (frame: { data: string; snapshot: boolean }) => void) => {
        listenCb = onFrame
        return () => {
          listenCb = null
        }
      },
    ),
    // Simulate a PTY output frame arriving from the daemon.
    deliver: (data: string, snapshot = false) => listenCb?.({ data, snapshot }),
    reset: () => {
      listenCb = null
    },
  }
})

vi.mock('@/lib/crowbar-bridge', () => ({
  terminalWrite: bridge.terminalWrite,
  terminalResize: bridge.terminalResize,
  terminalResync: bridge.terminalResync,
  terminalClose: bridge.terminalClose,
  terminalListen: bridge.terminalListen,
}))

vi.mock('@/extensions/themes/theme-registry', () => ({
  themeRegistry: { onThemeChange: () => () => {} },
}))

import { useTerminalConnection } from '@/features/terminal/hooks/use-terminal-connection'

// Minimal xterm stand-in covering only what the hook touches. write() invokes
// its parse-complete callback synchronously, mirroring xterm's contract closely
// enough to assert the finalize fires after the bulk replay is written.
function makeFakeTerminal() {
  // order records the relative sequence of reset() vs write(data) so the snapshot
  // sequencing (empty write's callback → reset → redraw write) can be asserted.
  const order: string[] = []
  const scrollToBottom = vi.fn()
  const refresh = vi.fn()
  const reset = vi.fn(() => order.push('reset'))
  const write = vi.fn((data: string, cb?: () => void) => {
    order.push(`write:${data}`)
    cb?.()
  })
  const disposable = () => ({ dispose: () => {} })
  const parent = { addEventListener: vi.fn(), removeEventListener: vi.fn() }
  let resizeCb: ((size: { cols: number; rows: number }) => void) | null = null
  const terminal = {
    rows: 40,
    write,
    reset,
    scrollToBottom,
    refresh,
    onData: vi.fn(disposable),
    onResize: vi.fn((cb: (size: { cols: number; rows: number }) => void) => {
      resizeCb = cb
      return { dispose: () => {} }
    }),
    onTitleChange: vi.fn(disposable),
    element: { parentElement: parent },
    buffer: { active: { type: 'normal' } },
    modes: { mouseTrackingMode: 'none', sendFocusMode: false },
  }
  return {
    terminal,
    scrollToBottom,
    refresh,
    reset,
    write,
    order,
    fireResize: (size: { cols: number; rows: number }) => resizeCb?.(size),
  }
}

function renderConnection(terminal: unknown, overrides: Record<string, unknown> = {}) {
  return renderHook(() =>
    useTerminalConnection({
      connectionId: 'conn-1',
      getTerminalTheme: () => ({}),
      isInitialized: true,
      reconnectKey: 0,
      sessionId: 'sess-1',
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      terminal: terminal as any,
      updateSession: () => {},
      ...overrides,
    }),
  )
}

describe('useTerminalConnection — re-attach viewport finalize', () => {
  // Collect rAF callbacks and flush them on demand so scheduleOutputFlush's
  // `outputFlushFrameRef = requestAnimationFrame(...)` assignment completes
  // BEFORE the flush runs (a synchronous rAF would null the ref out from under
  // the assignment and wedge every later flush).
  let rafCbs: FrameRequestCallback[] = []
  const flushRaf = () => {
    const cbs = rafCbs
    rafCbs = []
    for (const cb of cbs) cb(0)
  }

  beforeEach(() => {
    bridge.terminalListen.mockClear()
    bridge.terminalResync.mockClear()
    bridge.reset()
    rafCbs = []
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation((cb: FrameRequestCallback) => {
      rafCbs.push(cb)
      return rafCbs.length
    })
    vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => {})
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('repaints + scrolls to bottom once after the first post-attach flush', () => {
    const { terminal, scrollToBottom, refresh, write } = makeFakeTerminal()
    renderConnection(terminal)

    // First post-attach frame: the daemon's bulk scrollback replay.
    act(() => {
      bridge.deliver('REPLAYED SCROLLBACK')
      flushRaf()
    })

    expect(write).toHaveBeenCalledTimes(1)
    expect(write.mock.calls[0][0]).toBe('REPLAYED SCROLLBACK')
    expect(scrollToBottom).toHaveBeenCalledTimes(1)
    expect(refresh).toHaveBeenCalledTimes(1)
    // refresh must repaint every visible row (0..rows-1).
    expect(refresh).toHaveBeenCalledWith(0, 39)
  })

  it('does NOT re-run the expensive finalize on subsequent live output', () => {
    const { terminal, scrollToBottom, refresh, write } = makeFakeTerminal()
    renderConnection(terminal)

    act(() => {
      bridge.deliver('REPLAYED SCROLLBACK')
      flushRaf()
    })
    act(() => {
      bridge.deliver('live output chunk')
      flushRaf()
    })

    expect(write).toHaveBeenCalledTimes(2)
    // Finalize stays one-shot: streaming keeps xterm's cheap incremental render.
    expect(scrollToBottom).toHaveBeenCalledTimes(1)
    expect(refresh).toHaveBeenCalledTimes(1)
  })
})

describe('useTerminalConnection — snapshot frames (attach redraw / resize resync)', () => {
  let rafCbs: FrameRequestCallback[] = []
  const flushRaf = () => {
    const cbs = rafCbs
    rafCbs = []
    for (const cb of cbs) cb(0)
  }

  beforeEach(() => {
    bridge.terminalListen.mockClear()
    bridge.terminalResync.mockClear()
    bridge.reset()
    rafCbs = []
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation((cb: FrameRequestCallback) => {
      rafCbs.push(cb)
      return rafCbs.length
    })
    vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => {})
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('applies a snapshot onto a RESET buffer with the repaint finalize', () => {
    const { terminal, reset, scrollToBottom, refresh, order } = makeFakeTerminal()
    renderConnection(terminal)

    act(() => {
      bridge.deliver('CLEAN REDRAW', true)
    })

    expect(reset).toHaveBeenCalledTimes(1)
    // The reset + redraw are sequenced through an empty write's parse-complete
    // callback: empty write first, then reset, then the redraw write.
    expect(order).toEqual(['write:', 'reset', 'write:CLEAN REDRAW'])
    expect(scrollToBottom).toHaveBeenCalledTimes(1)
    expect(refresh).toHaveBeenCalledWith(0, 39)
  })

  it('drops buffered pre-snapshot output the redraw supersedes', () => {
    const { terminal, write } = makeFakeTerminal()
    renderConnection(terminal)

    act(() => {
      bridge.deliver('stale junk') // buffered, not yet flushed
      bridge.deliver('CLEAN REDRAW', true)
      flushRaf() // a stale scheduled flush must find an empty buffer
    })

    // Only the redraw reaches xterm as real content (the empty sequencing write
    // carries no data and is filtered out).
    const written = (write.mock.calls as [string][]).map(([d]) => d).filter((d) => d !== '')
    expect(written).toEqual(['CLEAN REDRAW'])
  })

  it('keeps delivering incremental output normally after a snapshot', () => {
    const { terminal, write, reset } = makeFakeTerminal()
    renderConnection(terminal)

    act(() => {
      bridge.deliver('CLEAN REDRAW', true)
    })
    act(() => {
      bridge.deliver('after')
      flushRaf()
    })

    const written = (write.mock.calls as [string][]).map(([d]) => d).filter((d) => d !== '')
    expect(written).toEqual(['CLEAN REDRAW', 'after'])
    expect(reset).toHaveBeenCalledTimes(1)
  })

  it('sequences reset+redraw through the write queue so queued live output cannot land after reset', () => {
    const { terminal, order } = makeFakeTerminal()
    renderConnection(terminal)

    act(() => {
      // Live output flushes into xterm's write queue BEFORE the snapshot arrives.
      bridge.deliver('live-1')
      flushRaf()
      // A snapshot arrives: reset+redraw must sequence THROUGH the write queue so
      // the already-queued 'live-1' parse completes before reset runs.
      bridge.deliver('CLEAN REDRAW', true)
    })

    const liveIdx = order.indexOf('write:live-1')
    const emptyIdx = order.indexOf('write:')
    const resetIdx = order.indexOf('reset')
    const redrawIdx = order.indexOf('write:CLEAN REDRAW')

    expect(liveIdx).toBeGreaterThanOrEqual(0)
    // The empty sequencing write is queued after the live write; its callback
    // (reset → redraw) therefore runs only after 'live-1' is parsed.
    expect(emptyIdx).toBeGreaterThan(liveIdx)
    expect(resetIdx).toBeGreaterThan(emptyIdx)
    expect(redrawIdx).toBeGreaterThan(resetIdx)
    // The snapshot redraw is the LAST write — no stale live bytes land after it.
    expect(redrawIdx).toBe(order.length - 1)
  })
})

describe('useTerminalConnection — debounced resize resync', () => {
  beforeEach(() => {
    bridge.terminalListen.mockClear()
    bridge.terminalResize.mockClear()
    bridge.terminalResync.mockClear()
    bridge.reset()
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('requests one resync after the last onResize of a gesture', () => {
    const { terminal, fireResize } = makeFakeTerminal()
    renderConnection(terminal)

    act(() => {
      fireResize({ cols: 100, rows: 30 })
      fireResize({ cols: 110, rows: 28 })
      fireResize({ cols: 120, rows: 25 })
    })
    expect(bridge.terminalResync).not.toHaveBeenCalled()

    act(() => {
      vi.advanceTimersByTime(300)
    })
    expect(bridge.terminalResync).toHaveBeenCalledTimes(1)
    expect(bridge.terminalResync).toHaveBeenCalledWith('conn-1')
    // Every resize still syncs the PTY dimensions immediately.
    expect(bridge.terminalResize).toHaveBeenCalledTimes(3)
  })
})
