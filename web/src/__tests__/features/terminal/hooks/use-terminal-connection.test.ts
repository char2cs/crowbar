import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { Terminal } from '@xterm/xterm'

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
  // order records reset() and write(data) in sequence, so a test can assert that a
  // snapshot never resets outside xterm's write queue.
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

// A snapshot is the daemon's whole screen: it lands as ONE write that begins with RIS, xterm's
// in-band reset, so the reset and the redraw parse in the same chunk and no frame shows between.
const RIS = '\x1bc'

describe('useTerminalConnection — snapshot frames (attach redraw / resize resync)', () => {
  it('writes a snapshot as one chunk that begins with RIS, with the repaint finalize', () => {
    const { terminal, reset, scrollToBottom, refresh, order } = makeFakeTerminal()
    renderConnection(terminal)

    act(() => {
      bridge.deliver('CLEAN REDRAW', true)
    })

    expect(order).toEqual([`write:${RIS}CLEAN REDRAW`])
    expect(reset).not.toHaveBeenCalled()
    expect(scrollToBottom).toHaveBeenCalledTimes(1)
    expect(refresh).toHaveBeenCalledWith(0, 39)
  })

  it('writes the frames around a snapshot in arrival order', () => {
    const { terminal, order } = makeFakeTerminal()
    renderConnection(terminal)

    act(() => {
      bridge.deliver('stale')
      bridge.deliver('S1', true)
      bridge.deliver('live')
      bridge.deliver('S2', true)
      bridge.deliver('after')
    })

    // The second snapshot parses last of the two, so its reset wins.
    expect(order).toEqual([
      'write:stale',
      `write:${RIS}S1`,
      'write:live',
      `write:${RIS}S2`,
      'write:after',
    ])
  })
})

// A real xterm whose every write slice expires (12 ms in WriteBuffer), so each chunk parses in its
// own timer task: what row 0 shows between two tasks is what a viewer could see.
function rowZeroAcrossTasks(...steps: ((reattach: () => void) => void)[]) {
  vi.useFakeTimers()
  let now = 0
  vi.spyOn(performance, 'now').mockImplementation(() => (now += 13))
  const terminal = new Terminal({ cols: 20, rows: 4, allowProposedApi: true })
  const { rerender } = renderConnectionRerenderable(terminal, {})
  const reattach = () => {
    bridge = makeFakeConnection()
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    rerender({ connection: bridge.conn as any })
  }
  for (const step of steps) act(() => step(reattach))
  const seen: string[] = []
  while (vi.getTimerCount() > 0) {
    act(() => {
      vi.advanceTimersToNextTimer()
    })
    seen.push(terminal.buffer.active.getLine(0)!.translateToString(true))
  }
  vi.useRealTimers()
  terminal.dispose()
  return seen
}

describe('useTerminalConnection — a snapshot on a real xterm', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('never shows a blank row between the old screen and the snapshot', () => {
    const seen = rowZeroAcrossTasks(() => {
      bridge.deliver('old')
      bridge.deliver('new', true)
    })
    expect(seen[0]).toBe('old')
    expect(seen.at(-1)).toBe('new')
    expect(seen).not.toContain('')
  })

  it("a torn-down connection's snapshot never lands after the new one's", () => {
    const seen = rowZeroAcrossTasks(
      () => bridge.deliver('first', true),
      (reattach) => reattach(),
      () => bridge.deliver('second', true),
    )
    expect(seen.at(-1)).toBe('second')
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
