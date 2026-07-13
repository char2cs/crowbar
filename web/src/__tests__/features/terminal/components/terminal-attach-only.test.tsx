import { createElement } from 'react'
import { act, render } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

// The transport-drop reconnect is the path that produced the live bug: the agent's
// PTY dies under a MOUNTED pane (daemon restart / CLI exit), the WS drops, and
// XtermTerminal re-resolves the connection. Without attach-only that re-resolve
// spawns a fresh shell into the agent's frame. These tests drive that exact path
// in the real component by firing the registered onTransportDrop callback.
const { resolveFn, terminalCreateFn, terminalDetachFn, dropCallbacks } = vi.hoisted(() => ({
  resolveFn: vi.fn(),
  terminalCreateFn: vi.fn(async (..._a: unknown[]) => 'fresh-shell'),
  terminalDetachFn: vi.fn(async (..._a: unknown[]) => {}),
  dropCallbacks: [] as Array<() => void>,
}))

vi.mock('@/features/terminal/components/resolve-terminal-connection', () => ({
  resolveTerminalConnection: (...a: unknown[]) => resolveFn(...a),
}))

vi.mock('@/lib/crowbar-bridge', () => ({
  terminalCreate: (...a: unknown[]) => terminalCreateFn(...a),
  terminalDetach: (...a: unknown[]) => terminalDetachFn(...a),
  terminalListLive: vi.fn(async () => [] as string[]),
  terminalResize: vi.fn(async () => {}),
  onTransportDrop: (_id: string, cb: () => void) => {
    dropCallbacks.push(cb)
    return () => {
      const i = dropCallbacks.indexOf(cb)
      if (i >= 0) dropCallbacks.splice(i, 1)
    }
  },
}))

vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getActiveWorkspaceId: () => 'w1',
}))

vi.mock('@/lib/workspace-scope-url', () => ({
  workspaceBase: (wsId: string) => `/v0/projects/p1/repos/r1/workspaces/${wsId}`,
}))

vi.mock('@/features/terminal/lib/terminal-reconnect-map', () => ({
  saveReconnect: vi.fn(),
  loadReconnect: vi.fn(() => null),
  clearReconnect: vi.fn(),
}))

// The connection hook owns the WS listener; the attach/spawn decision under test
// happens before it, so a passive stub keeps jsdom out of xterm's way.
vi.mock('@/features/terminal/hooks/use-terminal-connection', () => ({
  useTerminalConnection: () => ({
    currentConnectionIdRef: { current: null },
    writeBuffered: vi.fn(),
  }),
}))

import { XtermTerminal } from '@/features/terminal/components/terminal'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'

const SESSION = 'agent-term'

/**
 * Renders the terminal with a connectionId already in the store — the state a
 * mounted, attached agent pane is in. jsdom gives the container a 0×0 rect, so
 * xterm itself never initializes (nothing to render into); the transport-drop
 * subscription, which keys off the store's connectionId, is live regardless.
 * That is precisely the seam we need: the reconnect path, in the real component.
 */
async function renderAttached(props: { attachOnly?: boolean; onSessionGone?: () => void } = {}) {
  useTerminalStore.setState({
    sessions: new Map([[SESSION, { id: SESSION, connectionId: SESSION }]]),
  } as never)

  let unmount = () => {}
  await act(async () => {
    unmount = render(
      createElement(XtermTerminal, {
        sessionId: SESSION,
        isActive: true,
        isVisible: true,
        ...props,
      }),
    ).unmount
  })
  return { unmount }
}

async function fireTransportDrop() {
  await act(async () => {
    for (const cb of [...dropCallbacks]) cb()
  })
  await act(async () => {})
}

beforeEach(() => {
  resolveFn.mockReset()
  terminalCreateFn.mockClear()
  terminalDetachFn.mockClear()
  dropCallbacks.length = 0
  useTerminalStore.setState({ sessions: new Map() } as never)
})

// THE BLANK-CHAT BUG. An attach-only terminal is a VIEW onto a PTY someone else
// owns, and unlike a shell tab its xterm is NOT kept mounted (pane-container keeps
// only `terminal` buffers alive behind visibility:hidden) — it is destroyed and
// rebuilt whenever the chat tab is switched away from, closed and reopened, or the
// workspace is left and re-entered.
//
// The daemon serializes its screen model to a client at ATTACH and nowhere else
// (Session.Attach pre-fills the new client's channel with a ground-state redraw;
// Resync is gated on !isIdle and a command session's foreground process IS the CLI,
// so it reports idle and no-ops). So a transport that outlives its xterm is not a
// harmless optimisation — it makes the NEXT mount take the resolver's
// "reuse a live transport → no attach" branch, and the fresh, empty xterm is then
// never sent the screen. The agent is alive and typing into a terminal nobody
// redrew: the pane just sits blank until something unrelated (a pane resize →
// SIGWINCH → the CLI repaints itself) happens to bring it back. That is exactly the
// "sometimes the CLI doesn't come back" report — it came back only when the pane
// happened to change size.
//
// Releasing the transport with the xterm restores the invariant: no view, no
// connection — so every mount is an attach, and every attach is a redraw.
describe('XtermTerminal attach-only transport lifecycle', () => {
  it('releases its transport on unmount, so the next mount RE-ATTACHES and is redrawn', async () => {
    resolveFn.mockResolvedValue({ connectionId: SESSION, reused: true })
    const { unmount } = await renderAttached({ attachOnly: true })

    await act(async () => {
      unmount()
    })

    expect(terminalDetachFn).toHaveBeenCalledWith(SESSION)
  })

  it('NO REGRESSION: an ordinary terminal KEEPS its transport on unmount', async () => {
    // A shell tab's xterm is portaled and outlives pane splits and tab moves; its
    // socket must survive with it. Detaching here would drop a live shell's
    // transport on every layout change.
    resolveFn.mockResolvedValue({ connectionId: SESSION, reused: true })
    const { unmount } = await renderAttached() // no attachOnly — an ordinary shell tab

    await act(async () => {
      unmount()
    })

    expect(terminalDetachFn).not.toHaveBeenCalled()
  })
})

describe('XtermTerminal attach-only mode', () => {
  it('threads attachOnly into the transport-drop RECONNECT resolve', async () => {
    resolveFn.mockResolvedValue({ gone: true })
    await renderAttached({ attachOnly: true, onSessionGone: vi.fn() })

    expect(dropCallbacks.length).toBeGreaterThan(0) // subscribed to its connection
    await fireTransportDrop()

    expect(resolveFn).toHaveBeenCalled()
    const args = resolveFn.mock.calls.at(-1)?.[0] as { attachOnly?: boolean }
    expect(args.attachOnly).toBe(true)
  })

  it('does NOT spawn a shell and reports the session gone when the reconnect resolves GONE', async () => {
    resolveFn.mockResolvedValue({ gone: true })
    const onSessionGone = vi.fn()
    await renderAttached({ attachOnly: true, onSessionGone })

    await fireTransportDrop()

    // The live bug: this is where the bare shell used to be born.
    expect(terminalCreateFn).not.toHaveBeenCalled()
    expect(onSessionGone).toHaveBeenCalledTimes(1)
    // The dead id is never written back to the store as a live connection.
    expect(useTerminalStore.getState().getSession(SESSION)?.connectionId).toBe(SESSION)
  })

  it('still re-attaches on reconnect when the agent PTY survived', async () => {
    resolveFn.mockResolvedValue({ connectionId: SESSION, reused: true })
    const onSessionGone = vi.fn()
    await renderAttached({ attachOnly: true, onSessionGone })

    await fireTransportDrop()

    expect(onSessionGone).not.toHaveBeenCalled()
    expect(terminalCreateFn).not.toHaveBeenCalled()
    expect(useTerminalStore.getState().getSession(SESSION)?.connectionId).toBe(SESSION)
  })

  it('NO REGRESSION: an ordinary terminal pane reconnects WITHOUT attachOnly (still free to spawn)', async () => {
    resolveFn.mockResolvedValue({ connectionId: 'fresh-shell', reused: false })
    await renderAttached() // no attachOnly prop — an ordinary shell tab

    await fireTransportDrop()

    const args = resolveFn.mock.calls.at(-1)?.[0] as {
      attachOnly?: boolean
      createTerminal: () => Promise<string>
    }
    expect(args.attachOnly).toBe(false)
    // The resolver still gets a working createTerminal seam to spawn through.
    await expect(args.createTerminal()).resolves.toBe('fresh-shell')
    expect(terminalCreateFn).toHaveBeenCalled()
    // And the freshly-spawned connection is adopted by the tab.
    expect(useTerminalStore.getState().getSession(SESSION)?.connectionId).toBe('fresh-shell')
  })
})
