import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'

// A fake TerminalConnection the test drives by hand: the hook binds to whatever
// connection object it is given, so frames are delivered through its captured
// listener and everything the hook sends is recorded.
function makeFakeConnection() {
  type Frame = { data: Uint8Array; snapshot: boolean } | { exit: true; code: number }
  let listener: ((frame: Frame) => void) | null = null
  const conn = {
    alive: true,
    written: [] as string[],
    write: vi.fn((data: string) => {
      conn.written.push(data)
    }),
    resize: vi.fn(),
    setTheme: vi.fn(),
    close: vi.fn(),
    onDrop: vi.fn(() => () => {}),
    listen: vi.fn((cb: (frame: Frame) => void) => {
      listener = cb
      return () => {
        if (listener === cb) listener = null
      }
    }),
  }
  return {
    conn,
    // Simulate a PTY output frame arriving from the daemon.
    deliver: (data: string, snapshot = false) =>
      listener?.({ data: new TextEncoder().encode(data), snapshot }),
    // Simulate the daemon's exit frame.
    exit: (code: number) => listener?.({ exit: true, code }),
  }
}

// The connection each render binds to; beforeEach hooks replace it.
let bridge = makeFakeConnection()
beforeEach(() => {
  bridge = makeFakeConnection()
})

// xterm's write() takes a string or bytes; the fakes record both as text.
const asText = (data: string | Uint8Array) =>
  typeof data === 'string' ? data : new TextDecoder().decode(data)

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
  const write = vi.fn((data: string | Uint8Array, cb?: () => void) => {
    order.push(`write:${asText(data)}`)
    cb?.()
  })
  const disposable = () => ({ dispose: () => {} })
  const parent = { addEventListener: vi.fn(), removeEventListener: vi.fn() }
  let resizeCb: ((size: { cols: number; rows: number }) => void) | null = null
  let dataCb: ((data: string) => void) | null = null
  const terminal = {
    rows: 40,
    write,
    reset,
    scrollToBottom,
    refresh,
    onData: vi.fn((cb: (data: string) => void) => {
      dataCb = cb
      return { dispose: () => {} }
    }),
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
    type: (data: string) => dataCb?.(data),
  }
}

function renderConnection(terminal: unknown, overrides: Record<string, unknown> = {}) {
  return renderHook(() =>
    useTerminalConnection({
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      connection: bridge.conn as any,
      created: false,
      getTerminalTheme: () => ({}),
      sessionId: 'sess-1',
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      terminal: terminal as any,
      updateSession: () => {},
      ...overrides,
    }),
  )
}

describe('useTerminalConnection — re-attach viewport finalize', () => {
  beforeEach(() => {})

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('repaints + scrolls to bottom once after the first post-attach flush', () => {
    const { terminal, scrollToBottom, refresh, write } = makeFakeTerminal()
    renderConnection(terminal)

    // First post-attach frame: the daemon's bulk scrollback replay. Written
    // to xterm synchronously on arrival — no discretionary rAF delay.
    act(() => {
      bridge.deliver('REPLAYED SCROLLBACK')
    })

    expect(write).toHaveBeenCalledTimes(1)
    expect(asText(write.mock.calls[0][0])).toBe('REPLAYED SCROLLBACK')
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
    })
    act(() => {
      bridge.deliver('live output chunk')
    })

    expect(write).toHaveBeenCalledTimes(2)
    // Finalize stays one-shot: streaming keeps xterm's cheap incremental render.
    expect(scrollToBottom).toHaveBeenCalledTimes(1)
    expect(refresh).toHaveBeenCalledTimes(1)
  })

  it('writes an incremental frame synchronously — no discretionary rAF in between', () => {
    const rafSpy = vi.spyOn(window, 'requestAnimationFrame').mockImplementation(() => 0)
    const { terminal, write } = makeFakeTerminal()
    renderConnection(terminal)

    // No follow-up rAF flush anywhere: the write must already be visible the
    // instant deliver() returns, inside the same act().
    act(() => {
      bridge.deliver('incremental chunk')
    })

    expect(asText(write.mock.calls[0][0])).toBe('incremental chunk')
    expect(write.mock.calls[0][1]).toEqual(expect.any(Function))
    expect(rafSpy).not.toHaveBeenCalled()
  })
})

describe('useTerminalConnection — snapshot frames (attach redraw / resize resync)', () => {
  beforeEach(() => {})

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

  it('supersedes pre-snapshot output with the reset + redraw', () => {
    const { terminal, order, scrollToBottom, refresh } = makeFakeTerminal()
    renderConnection(terminal)

    act(() => {
      // Written immediately on arrival — no discretionary delay left to hold
      // it back — but a snapshot arriving right after still wipes it out.
      bridge.deliver('stale junk')
      bridge.deliver('CLEAN REDRAW', true)
    })

    // The pre-snapshot write lands, but the barrier's reset()+redraw run
    // strictly after it and are the terminal's final, definitive state.
    expect(order).toEqual(['write:stale junk', 'write:', 'reset', 'write:CLEAN REDRAW'])
    expect(scrollToBottom).toHaveBeenCalled()
    expect(refresh).toHaveBeenCalledWith(0, 39)
  })

  it('keeps delivering incremental output normally after a snapshot', () => {
    const { terminal, write, reset } = makeFakeTerminal()
    renderConnection(terminal)

    act(() => {
      bridge.deliver('CLEAN REDRAW', true)
    })
    act(() => {
      bridge.deliver('after')
    })

    const written = (write.mock.calls as [string | Uint8Array][])
      .map(([d]) => asText(d))
      .filter((d) => d !== '')
    expect(written).toEqual(['CLEAN REDRAW', 'after'])
    expect(reset).toHaveBeenCalledTimes(1)
  })

  it('sequences reset+redraw through the write queue so queued live output cannot land after reset', () => {
    const { terminal, order } = makeFakeTerminal()
    renderConnection(terminal)

    act(() => {
      // Live output is written immediately — no discretionary delay.
      bridge.deliver('live-1')
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
  const write = vi.fn((raw: string | Uint8Array, cb?: () => void) => {
    const data = asText(raw)
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
  beforeEach(() => {})

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('holds post-snapshot frames until the barrier callback actually runs, never parsing them between reset and the redraw', () => {
    const { terminal, write, order, drainWrites } = makeAsyncFakeTerminal()
    renderConnection(terminal)

    // D0 lands and is handed to xterm immediately (no discretionary delay),
    // but xterm has not parsed it yet — it sits enqueued in the async write
    // queue (undrained backlog), exactly like a real xterm under load.
    act(() => {
      bridge.deliver('D0')
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

    // D1 arrives while the snapshot latch (snapshotPendingRef) is set. It
    // must NOT reach terminal.write at all — it must stay buffered in
    // outputBufferRef.
    act(() => {
      bridge.deliver('D1')
    })
    expect(write).not.toHaveBeenCalledWith('D1', expect.anything())
    expect(order).toEqual(['enqueue:D0', 'enqueue:'])

    // Drain step by step: first D0 parses (stale, pre-barrier content), then
    // the empty barrier write parses, running its callback synchronously,
    // which calls reset(), enqueues the redraw write (SNAP), clears the
    // latch, and — with the discretionary rAF gone — drains the buffered D1
    // synchronously in that same callback, enqueuing its write right behind
    // the redraw. No further tick is needed.
    drainWrites()

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

// Renders with rerenderable props so tests can hand in a NEW connection mid-test
// — a transport-drop re-attach — forcing the main effect to tear down and re-run
// on the SAME terminal instance, unlike renderConnection() above which fixes all
// props for the hook's lifetime.
function renderConnectionRerenderable(
  terminal: unknown,
  initialOverrides: Record<string, unknown> = {},
) {
  return renderHook(
    (props: Record<string, unknown>) =>
      useTerminalConnection({
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        connection: bridge.conn as any,
        created: false,
        getTerminalTheme: () => ({}),
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
  beforeEach(() => {})

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
    // redraw write. With the discretionary rAF gone, the buffered increment
    // is drained synchronously in that same callback, right behind S2 — no
    // extra tick needed.
    drainWrites()

    expect(reset).toHaveBeenCalledTimes(1)
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

  it('stale barrier across a re-attach (a new connection) never fires against the new connection', () => {
    const { terminal, reset, order, drainWrites } = makeAsyncFakeTerminal()
    const { rerender } = renderConnectionRerenderable(terminal, {})

    act(() => {
      // S1 latches on the FIRST effect instance and its barrier is enqueued —
      // left undrained across the reconnect below.
      bridge.deliver('S1', true)
    })
    expect(order).toEqual(['enqueue:'])

    // Simulate a transport-drop re-attach: a new connection is handed in, the
    // main effect tears down (bumping the generation) and re-runs, listening on
    // the new connection with the same terminal instance.
    bridge = makeFakeConnection()
    act(() => {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      rerender({ connection: bridge.conn as any })
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

describe('useTerminalConnection — theme propagation', () => {
  beforeEach(() => {
    themeReg.reset()
  })

  afterEach(() => {
    themeReg.reset()
  })

  it('pushes the current theme to the daemon on attach', () => {
    const { terminal } = makeFakeTerminal()
    renderConnection(terminal)

    expect(bridge.conn.setTheme).toHaveBeenCalledWith(
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

    act(() => {
      themeReg.fire()
    })

    expect(bridge.conn.setTheme).toHaveBeenCalledTimes(2) // attach + switch
  })
})

// P0-6 / B4: the daemon's exit frame is the only thing that ends a terminal. Typing
// "exit" is ordinary input — inside ssh, a REPL or an agent TUI it ends nothing.
describe('useTerminalConnection — exit is daemon-authoritative', () => {
  beforeEach(() => {})

  it('forwards a typed "exit" verbatim and does not end the terminal', async () => {
    vi.useFakeTimers()
    const onTerminalExit = vi.fn()
    const { terminal, type } = makeFakeTerminal()
    renderConnection(terminal, { onTerminalExit })

    act(() => {
      type('exit')
      type('\r')
    })
    await act(async () => {
      await Promise.resolve()
    })

    await act(async () => {
      await vi.runAllTimersAsync()
    })
    vi.useRealTimers()

    expect(bridge.conn.written.join('')).toBe('exit\r')
    expect(bridge.conn.close).not.toHaveBeenCalled()
    expect(onTerminalExit).not.toHaveBeenCalled()
  })

  it('ends the terminal when the daemon sends its exit frame', () => {
    const onTerminalExit = vi.fn()
    const { terminal } = makeFakeTerminal()
    renderConnection(terminal, { onTerminalExit })

    act(() => {
      bridge.deliver('bye')
      bridge.exit(0)
    })

    expect(onTerminalExit).toHaveBeenCalledExactlyOnceWith('sess-1')
  })
})

describe('useTerminalConnection — initial command', () => {
  it("runs a fresh shell's initial command once, on its first frame — no timer", () => {
    const { terminal } = makeFakeTerminal()
    const { rerender } = renderConnectionRerenderable(terminal, {
      created: true,
      initialCommand: 'npm test',
    })
    expect(bridge.conn.written).toEqual([])
    act(() => {
      bridge.deliver('PROMPT$ ', true)
      bridge.deliver('more')
    })
    // A re-render that re-binds the same connection must not send it again.
    act(() => {
      rerender({ created: true, initialCommand: 'npm test', updateSession: () => {} })
    })
    act(() => {
      bridge.deliver('even more')
    })
    expect(bridge.conn.written).toEqual(['npm test\n'])
  })

  it('never runs it on a session this view merely re-attached to', () => {
    const { terminal } = makeFakeTerminal()
    renderConnection(terminal, { created: false, initialCommand: 'npm test' })
    act(() => {
      bridge.deliver('PROMPT$ ', true)
    })
    expect(bridge.conn.written).toEqual([])
  })
})

describe('useTerminalConnection — input across a re-attach', () => {
  it('holds keystrokes typed with no connection and sends them, in order, once one arrives', () => {
    const { terminal, type } = makeFakeTerminal()
    const first = bridge
    const { rerender } = renderConnectionRerenderable(terminal, {})
    act(() => {
      rerender({ connection: null })
    })
    act(() => {
      type('ls')
      type('\r')
    })
    expect(first.conn.written).toEqual([])
    bridge = makeFakeConnection()
    act(() => {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      rerender({ connection: bridge.conn as any })
    })
    expect(bridge.conn.written).toEqual(['ls\r'])
  })
})
