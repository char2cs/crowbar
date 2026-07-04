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
    terminalSetTheme: vi.fn(async () => {}),
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
  terminalSetTheme: bridge.terminalSetTheme,
  terminalClose: bridge.terminalClose,
  terminalListen: bridge.terminalListen,
}))

// Capturing themeRegistry mock: fire() invokes the hook's registered onThemeChange
// callback so a test can simulate a light<->dark switch.
const themeReg = vi.hoisted(() => {
  let cb: (() => void) | null = null
  return {
    onThemeChange: (fn: () => void) => {
      cb = fn
      return () => {
        cb = null
      }
    },
    fire: () => cb?.(),
    reset: () => {
      cb = null
    },
  }
})

vi.mock('@/extensions/themes/theme-registry', () => ({
  themeRegistry: { onThemeChange: themeReg.onThemeChange },
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
    // The onThemeChange handler assigns terminal.options.theme; give it a home.
    options: {} as { theme?: unknown },
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

// Async fake terminal: write(data, cb) does NOT invoke cb synchronously.
// Instead it pushes {data, cb} onto a queue that the test drains explicitly
// via drainWrites(), one entry at a time — mirroring xterm's real contract
// where write() schedules an async parse and the callback fires only once
// that parse completes. The synchronous fake above is vacuous for ordering
// bugs where a rAF-flushed frame can be enqueued BETWEEN the barrier write
// and its callback; this fake makes that race observable.
function makeAsyncFakeTerminal() {
  const order: string[] = []
  type QueueEntry = { data: string; cb?: () => void }
  const queue: QueueEntry[] = []
  const scrollToBottom = vi.fn()
  const refresh = vi.fn()
  const reset = vi.fn(() => order.push('reset'))
  const write = vi.fn((data: string, cb?: () => void) => {
    order.push(`enqueue:${data}`)
    queue.push({ data, cb })
  })
  // Drains only the entries present in the queue AT CALL TIME — a callback
  // invoked mid-drain can itself enqueue new writes (e.g. the snapshot barrier
  // enqueuing the redraw), and those must wait for the NEXT drainWrites() call
  // rather than being swept up in this one. That's what makes "step by step"
  // draining actually observe the enqueue-vs-parse ordering.
  const drainWrites = () => {
    const batch = queue.splice(0, queue.length)
    for (const entry of batch) {
      order.push(`parse:${entry.data}`)
      entry.cb?.()
    }
  }
  const disposable = () => ({ dispose: () => {} })
  const parent = { addEventListener: vi.fn(), removeEventListener: vi.fn() }
  const terminal = {
    rows: 40,
    write,
    reset,
    scrollToBottom,
    refresh,
    onData: vi.fn(disposable),
    onResize: vi.fn(() => ({ dispose: () => {} })),
    onTitleChange: vi.fn(disposable),
    element: { parentElement: parent },
    buffer: { active: { type: 'normal' } },
    modes: { mouseTrackingMode: 'none', sendFocusMode: false },
    // The onThemeChange handler assigns terminal.options.theme; give it a home.
    options: {} as { theme?: unknown },
  }
  return { terminal, write, reset, order, drainWrites, queueLength: () => queue.length }
}

describe('useTerminalConnection — snapshot latch vs async xterm write queue', () => {
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

  it('holds post-snapshot frames until the barrier callback actually runs, never parsing them between reset and the redraw', () => {
    const { terminal, write, order, drainWrites } = makeAsyncFakeTerminal()
    renderConnection(terminal)

    // D0 lands and gets rAF-flushed, but xterm has not parsed it yet — it sits
    // enqueued in the async write queue (undrained backlog), exactly like a
    // real xterm under load.
    act(() => {
      bridge.deliver('D0')
      flushRaf()
    })
    expect(order).toEqual(['enqueue:D0'])

    // Snapshot arrives while D0 is still unparsed.
    act(() => {
      bridge.deliver('SNAP', true)
    })
    // The barrier write is enqueued behind D0; nothing has parsed yet, so
    // reset()/the redraw write must not have happened.
    expect(order).toEqual(['enqueue:D0', 'enqueue:'])
    expect(reset_not_called(terminal)).toBe(true)

    // D1 arrives and is rAF-flushed BEFORE anything is drained. Because the
    // snapshot latch (snapshotPendingRef) is set, this must NOT reach
    // terminal.write at all — it must stay buffered in outputBufferRef.
    act(() => {
      bridge.deliver('D1')
      flushRaf()
    })
    expect(write).not.toHaveBeenCalledWith('D1', expect.anything())
    expect(order).toEqual(['enqueue:D0', 'enqueue:'])

    // Drain step by step: first D0 parses (stale, pre-barrier content), then
    // the empty barrier write parses, running its callback synchronously,
    // which calls reset(), enqueues the redraw write (SNAP), clears the latch,
    // and reschedules a flush for the buffered D1 (via rAF — not synchronous).
    drainWrites()

    expect(order).toEqual(['enqueue:D0', 'enqueue:', 'parse:D0', 'parse:', 'reset', 'enqueue:SNAP'])

    // The rescheduled flush actually enqueues D1's write only once its rAF
    // fires — and only AFTER the redraw (SNAP) was already enqueued above.
    act(() => {
      flushRaf()
    })
    expect(order).toEqual([
      'enqueue:D0',
      'enqueue:',
      'parse:D0',
      'parse:',
      'reset',
      'enqueue:SNAP',
      'enqueue:D1',
    ])

    // Draining the rest parses SNAP then D1, in that order — D1 parses only
    // AFTER the redraw, never between reset and SNAP.
    drainWrites()
    expect(order).toEqual([
      'enqueue:D0',
      'enqueue:',
      'parse:D0',
      'parse:',
      'reset',
      'enqueue:SNAP',
      'enqueue:D1',
      'parse:SNAP',
      'parse:D1',
    ])
  })
})

function reset_not_called(terminal: { reset: ReturnType<typeof vi.fn> }) {
  return terminal.reset.mock.calls.length === 0
}

// Renders with rerenderable props so tests can bump reconnectKey mid-test to
// force the main effect to tear down and re-run on the SAME terminal instance
// (simulating a transport-drop re-attach), unlike renderConnection() above
// which fixes all props for the hook's lifetime.
function renderConnectionRerenderable(
  terminal: unknown,
  initialOverrides: Record<string, unknown> = {},
) {
  return renderHook(
    (props: Record<string, unknown>) =>
      useTerminalConnection({
        connectionId: 'conn-1',
        getTerminalTheme: () => ({}),
        isInitialized: true,
        reconnectKey: 0,
        sessionId: 'sess-1',
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        terminal: terminal as any,
        updateSession: () => {},
        ...props,
      }),
    { initialProps: initialOverrides },
  )
}

describe('useTerminalConnection — generation-guarded snapshot barrier (R3-1)', () => {
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

  it('double snapshot: only the SECOND snapshot resets+redraws; the first barrier is dead', () => {
    const { terminal, reset, order, drainWrites } = makeAsyncFakeTerminal()
    renderConnection(terminal)

    act(() => {
      // S1 latches and enqueues its barrier — undrained.
      bridge.deliver('S1', true)
      // S2 arrives before S1's barrier callback has run: re-latches, enqueues
      // a second barrier.
      bridge.deliver('S2', true)
      // An increment delivered while still latched must stay buffered and can
      // only reach xterm after S2's redraw is enqueued.
      bridge.deliver('INC')
    })
    // Both barriers enqueued, nothing parsed yet.
    expect(order).toEqual(['enqueue:', 'enqueue:'])

    // Drain everything currently queued: both barrier callbacks fire. The
    // first (stale) must be a no-op; only the second may reset + enqueue the
    // redraw write.
    drainWrites()

    expect(reset).toHaveBeenCalledTimes(1)
    expect(order).toEqual(['enqueue:', 'enqueue:', 'parse:', 'parse:', 'reset', 'enqueue:S2'])

    // The buffered increment reschedules its own flush via rAF (not
    // synchronously) once the live latch clears.
    act(() => {
      flushRaf()
    })
    expect(order).toEqual([
      'enqueue:',
      'enqueue:',
      'parse:',
      'parse:',
      'reset',
      'enqueue:S2',
      'enqueue:INC',
    ])

    // Draining the rest parses the redraw (S2) then the increment, in order —
    // S1's content never reaches xterm at all.
    drainWrites()
    expect(order).toEqual([
      'enqueue:',
      'enqueue:',
      'parse:',
      'parse:',
      'reset',
      'enqueue:S2',
      'enqueue:INC',
      'parse:S2',
      'parse:INC',
    ])
  })

  it('stale barrier across an effect re-run (reconnectKey bump) never fires against the new connection', () => {
    const { terminal, reset, order, drainWrites } = makeAsyncFakeTerminal()
    const { rerender } = renderConnectionRerenderable(terminal, { reconnectKey: 0 })

    act(() => {
      // S1 latches on the FIRST effect instance and its barrier is enqueued —
      // left undrained across the reconnect below.
      bridge.deliver('S1', true)
    })
    expect(order).toEqual(['enqueue:'])

    // Simulate a transport-drop re-attach: reconnectKey bumps, the main
    // effect tears down (bumping the generation + clearing the latch) and
    // re-runs, re-registering terminalListen on the same terminal instance.
    act(() => {
      rerender({ reconnectKey: 1 })
    })

    act(() => {
      // S2 latches on the NEW effect instance/generation.
      bridge.deliver('S2', true)
    })
    expect(order).toEqual(['enqueue:', 'enqueue:'])

    // Drain everything: S1's stale barrier must be inert (no reset, no
    // write), and must not have unlatched/interfered with S2's barrier.
    drainWrites()

    expect(reset).toHaveBeenCalledTimes(1)
    expect(order).toEqual(['enqueue:', 'enqueue:', 'parse:', 'parse:', 'reset', 'enqueue:S2'])

    drainWrites()
    expect(order).toEqual([
      'enqueue:',
      'enqueue:',
      'parse:',
      'parse:',
      'reset',
      'enqueue:S2',
      'parse:S2',
    ])
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

describe('useTerminalConnection — theme propagation', () => {
  beforeEach(() => {
    bridge.terminalSetTheme.mockClear()
    themeReg.reset()
  })

  afterEach(() => {
    themeReg.reset()
  })

  it('pushes the current theme to the daemon on attach', () => {
    const { terminal } = makeFakeTerminal()
    renderConnection(terminal)

    expect(bridge.terminalSetTheme).toHaveBeenCalledWith(
      'conn-1',
      expect.objectContaining({
        background: expect.any(String),
        foreground: expect.any(String),
        dark: expect.any(Boolean),
      }),
    )
  })

  it('re-pushes the theme when the app theme switches', () => {
    const { terminal } = makeFakeTerminal()
    renderConnection(terminal)
    bridge.terminalSetTheme.mockClear()

    act(() => {
      themeReg.fire()
    })

    expect(bridge.terminalSetTheme).toHaveBeenCalledTimes(1)
    expect(bridge.terminalSetTheme).toHaveBeenCalledWith('conn-1', expect.any(Object))
  })
})
