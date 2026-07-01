import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'

// vi.hoisted runs before the vi.mock factories below, so the bridge spies and
// the captured listen callback exist when the mock module is constructed.
const bridge = vi.hoisted(() => {
  let listenCb: ((data: string) => void) | null = null
  return {
    terminalWrite: vi.fn(async () => {}),
    terminalResize: vi.fn(async () => {}),
    terminalClose: vi.fn(async () => {}),
    terminalListen: vi.fn((_id: string, onData: (data: string) => void) => {
      listenCb = onData
      return () => {
        listenCb = null
      }
    }),
    // Simulate a PTY output frame arriving from the daemon.
    deliver: (data: string) => listenCb?.(data),
    reset: () => {
      listenCb = null
    },
  }
})

vi.mock('@/lib/crowbar-bridge', () => ({
  terminalWrite: bridge.terminalWrite,
  terminalResize: bridge.terminalResize,
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
  const scrollToBottom = vi.fn()
  const refresh = vi.fn()
  const write = vi.fn((_data: string, cb?: () => void) => {
    cb?.()
  })
  const disposable = () => ({ dispose: () => {} })
  const parent = { addEventListener: vi.fn(), removeEventListener: vi.fn() }
  const terminal = {
    rows: 40,
    write,
    scrollToBottom,
    refresh,
    onData: vi.fn(disposable),
    onResize: vi.fn(disposable),
    onTitleChange: vi.fn(disposable),
    element: { parentElement: parent },
    buffer: { active: { type: 'normal' } },
    modes: { mouseTrackingMode: 'none', sendFocusMode: false },
  }
  return { terminal, scrollToBottom, refresh, write }
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
