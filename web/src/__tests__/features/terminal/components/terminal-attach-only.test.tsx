import { createElement, StrictMode } from 'react'
import { act, render } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

// The transport-drop reconnect is the path that produced the live bug: the agent's
// PTY dies under a MOUNTED pane (daemon restart / CLI exit), the WS drops, and
// XtermTerminal re-resolves the connection. Without attach-only that re-resolve
// spawns a fresh shell into the agent's frame. These tests drive that exact path
// in the real component by firing the registered onTransportDrop callback.
const {
  resolveFn,
  terminalCreateFn,
  terminalDetachFn,
  dropCallbacks,
  connectionCalls,
  injectLinkStylesFn,
  removeLinkStylesFn,
} = vi.hoisted(() => ({
  resolveFn: vi.fn(),
  terminalCreateFn: vi.fn(async (..._a: unknown[]) => 'fresh-shell'),
  terminalDetachFn: vi.fn(async (..._a: unknown[]) => {}),
  dropCallbacks: [] as Array<() => void>,
  // Every render's useTerminalConnection inputs, in order. The last entry is what
  // the connection effect keys its terminalListen on — the re-listen contract.
  connectionCalls: [] as Array<{ connectionId?: string; reconnectKey?: number }>,
  injectLinkStylesFn: vi.fn(),
  removeLinkStylesFn: vi.fn(),
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

// NOT mocked: the REAL chat-scoped URL builder runs. w1's PTY base resolves
// through the chat recorded for it in beforeEach (`/v0/chats/chat-w1/terminals`).

vi.mock('@/features/terminal/lib/terminal-reconnect-map', () => ({
  saveReconnect: vi.fn(),
  loadReconnect: vi.fn(() => null),
  clearReconnect: vi.fn(),
}))

// The connection hook owns the WS listener; the attach/spawn decision under test
// happens before it, so a passive stub keeps jsdom out of xterm's way. It CAPTURES
// its per-render inputs, because those inputs ARE the component→hook contract the
// swap must drive: the real hook re-runs its listen effect on [connectionId,
// reconnectKey], so "last call saw the new id with a bumped key" is exactly
// "the listener re-registered on the fresh transport" (the bump path itself is
// proven at hook level in use-terminal-connection.test.ts).
vi.mock('@/features/terminal/hooks/use-terminal-connection', () => ({
  useTerminalConnection: (opts: { connectionId?: string; reconnectKey?: number }) => {
    connectionCalls.push({ connectionId: opts.connectionId, reconnectKey: opts.reconnectKey })
    return {
      currentConnectionIdRef: { current: null },
      writeBuffered: vi.fn(),
    }
  },
}))

// Link styles are injected per session (idempotent, keyed by style-element id) and
// removed by the init effect's sessionId-keyed cleanup. On a swap the cleanup runs
// for the OLD session and initializeTerminal — the only other injector — never runs
// again, so the swap path itself must re-inject for the incoming session.
vi.mock('@/features/terminal/hooks/use-terminal-addons', () => ({
  createTerminalAddons: vi.fn(),
  injectLinkStyles: (...a: unknown[]) => injectLinkStylesFn(...a),
  loadWebLinksAddon: vi.fn(),
  removeLinkStyles: (...a: unknown[]) => removeLinkStylesFn(...a),
}))

import { XtermTerminal } from '@/features/terminal/components/terminal'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'
import { __resetAttachRefcountForTest } from '@/features/terminal/lib/attach-refcount'
import { recordWorkspaceScope, __resetWorkspaceScopesForTest } from '@/lib/workspace-scope'

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

// The agent chat pane points ONE mounted terminal at a succession of PTYs: an
// external provider switch or a revive lands a NEW live runner on the chat, and the
// pane feeds its new terminalSessionId in as a PROP CHANGE — the key= that used to
// force a full remount is gone. Seed ALL the sessions a test may swap through so
// each incoming one's connectionId can be resolved out of the store, exactly as the
// pane's seedAttach leaves it before flipping the prop.
async function renderSwappable(sessionId: string, attachOnly = true) {
  useTerminalStore.setState({
    sessions: new Map([
      ['A', { id: 'A', connectionId: 'A' }],
      ['B', { id: 'B', connectionId: 'B' }],
      ['C', { id: 'C', connectionId: 'C' }],
    ]),
  } as never)

  const onSessionGone = vi.fn()
  let result!: ReturnType<typeof render>
  await act(async () => {
    result = render(
      createElement(XtermTerminal, {
        sessionId,
        isActive: true,
        isVisible: true,
        attachOnly,
        onSessionGone,
      }),
    )
  })
  const rerender = async (nextSessionId: string) => {
    await act(async () => {
      result.rerender(
        createElement(XtermTerminal, {
          sessionId: nextSessionId,
          isActive: true,
          isVisible: true,
          attachOnly,
          onSessionGone,
        }),
      )
    })
    // Settle doReconnect's awaited resolve before the assertions read the spies.
    await act(async () => {})
  }
  return { result, rerender, onSessionGone }
}

beforeEach(() => {
  resolveFn.mockReset()
  terminalCreateFn.mockClear()
  terminalDetachFn.mockClear()
  injectLinkStylesFn.mockClear()
  removeLinkStylesFn.mockClear()
  dropCallbacks.length = 0
  connectionCalls.length = 0
  useTerminalStore.setState({ sessions: new Map() } as never)
  // The attach refcount ledger is process-global (a property of the daemon
  // connection, not of any React tree) — clear it so counts a prior test retained
  // don't suppress a later test's detach.
  __resetAttachRefcountForTest()
  // PTY routes are chat-scoped: the terminal resolves w1's base through the chat
  // that owns w1's worktree, so the scope must carry an owningChatId or the real
  // builder throws.
  __resetWorkspaceScopesForTest()
  recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'w1', owningChatId: 'chat-w1' })
})

/** A promise the test resolves by hand — the racing-swap tests hold a resolve in
 *  flight and drive its completion themselves. No timers, ever. */
function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((res) => {
    resolve = res
  })
  return { promise, resolve }
}

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

  it('conserves the attach count across a transport-drop RECONNECT, so unmount still detaches once', async () => {
    // The refcount is process-global and keyed by connectionId. Init retains once
    // (count→1); a transport-drop reconnect used to retain AGAIN with NO matching
    // release. On a plain drop the sessionId AND connectionId are unchanged, so
    // neither the swap reconciler nor the unmount effect releases — the count
    // leaked 1→2. The LAST-releaser detach on unmount then saw 2→1 (false) and was
    // SUPPRESSED, leaving both the transport AND the resolver's
    // terminalHasTransport(connId) alive → the next mount short-circuits reuse
    // without re-attaching → BLANK PANE (the bug this branch exists to fix).
    //
    // jsdom gives the container a 0×0 rect, so initializeTerminal (the init retain)
    // never runs. The FIRST drop below stands in for that init retain (count→1); the
    // SECOND is the same-id re-attach that used to double-count. The fix conserves
    // the ledger in doReconnect, so the count returns to 1 and unmount detaches.
    resolveFn.mockResolvedValue({ connectionId: SESSION, reused: true })
    const { unmount } = await renderAttached({ attachOnly: true })

    await fireTransportDrop() // stands in for the init retain: count → 1
    await fireTransportDrop() // same-id re-attach: must NET the count, not leak to 2

    // Neither reconnect detaches anything (same-id: reconciler no-ops, no swap).
    expect(terminalDetachFn).not.toHaveBeenCalled()

    await act(async () => {
      unmount()
    })

    // The count returned to 1, so unmount's release hit 0 and performed THE detach —
    // exactly once. Pre-fix the count is 2 here and this detach never fires.
    expect(terminalDetachFn).toHaveBeenCalledTimes(1)
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
    // A survived PTY reconnects on a NEW ws connection: same session id, DIFFERENT
    // connectionId. Resolving to a distinct id from the one renderAttached seeded is what
    // makes the re-attach write-back observable — asserting the seeded id was unchanged
    // (as this test used to) passes even if the entire updateSession/saveReconnect path is
    // deleted, because the seed and the resolve were the same string.
    resolveFn.mockResolvedValue({ connectionId: 'reattached-conn', reused: true })
    const onSessionGone = vi.fn()
    await renderAttached({ attachOnly: true, onSessionGone })

    await fireTransportDrop()

    expect(onSessionGone).not.toHaveBeenCalled()
    expect(terminalCreateFn).not.toHaveBeenCalled()
    // The write-back RAN: the surviving session now points at the re-attached connection.
    expect(useTerminalStore.getState().getSession(SESSION)?.connectionId).toBe('reattached-conn')
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

// P4c — REATTACH WITHOUT REMOUNT. A provider switch or a chat revive replaces the
// agent's runner, giving the chat a NEW terminalSessionId. The pane used to force
// that through with key={sessionId}, which unmounted the ENTIRE terminal — socket,
// xterm instance, observers, listeners — and rebuilt it. It now feeds the new id in
// as a plain prop change, and the terminal swaps only the TRANSPORT: it releases the
// outgoing PTY (detach) and attaches the incoming one THROUGH the same resolver the
// transport-drop path uses (so it can never spawn a bare shell), keeping the xterm
// instance — and everything hung off it — alive.
describe('XtermTerminal attach-only imperative reattach (session swap without remount)', () => {
  it('swaps the attachment on a sessionId change: 1 detach of the old, 1 resolver-attach of the new, SAME DOM node', async () => {
    resolveFn.mockResolvedValue({ connectionId: 'B', reused: true })
    const { result, rerender, onSessionGone } = await renderSwappable('A')

    const before = result.container.querySelector('.xterm-container')
    expect(before).not.toBeNull()

    await rerender('B')

    // Exactly one close of the outgoing PTY...
    expect(terminalDetachFn).toHaveBeenCalledTimes(1)
    expect(terminalDetachFn).toHaveBeenCalledWith('A')
    // ...and exactly one open of the incoming one, through the resolver — attachOnly,
    // so it resolves-or-reports-gone and can NEVER reach createTerminal.
    expect(resolveFn).toHaveBeenCalledTimes(1)
    const args = resolveFn.mock.calls.at(-1)?.[0] as {
      attachOnly?: boolean
      storeConnectionId?: string
      tabSessionId?: string
    }
    expect(args.attachOnly).toBe(true)
    expect(args.storeConnectionId).toBe('B')
    expect(args.tabSessionId).toBe('B')
    expect(terminalCreateFn).not.toHaveBeenCalled() // never a bare shell
    expect(onSessionGone).not.toHaveBeenCalled()

    // THE HEADLINE: the terminal component was NOT remounted. The very same xterm
    // container DOM node — so its xterm instance, WS listener and observers all
    // survived the switch.
    const after = result.container.querySelector('.xterm-container')
    expect(after).toBe(before)
    // The store now points the incoming session at its resolved connection.
    expect(useTerminalStore.getState().getSession('B')?.connectionId).toBe('B')
  })

  it('does nothing when the sessionId prop is unchanged (an unrelated parent re-render)', async () => {
    resolveFn.mockResolvedValue({ connectionId: 'A', reused: true })
    const { rerender } = await renderSwappable('A')

    await rerender('A')

    expect(terminalDetachFn).not.toHaveBeenCalled()
    expect(resolveFn).not.toHaveBeenCalled()
  })

  it('reports the session gone (never spawns) when the incoming PTY is not live', async () => {
    resolveFn.mockResolvedValue({ gone: true })
    const { rerender, onSessionGone } = await renderSwappable('A')

    await rerender('B')

    expect(onSessionGone).toHaveBeenCalledTimes(1)
    expect(terminalCreateFn).not.toHaveBeenCalled()
  })

  it('NO REGRESSION: an ordinary (non-attachOnly) terminal does not imperatively reattach on a sessionId change', async () => {
    // Shell tabs never swap sessionId under a mounted xterm, and their portaled
    // socket must survive layout changes. The swap is gated on attachOnly so this
    // path is untouched — no detach, no re-resolve.
    resolveFn.mockResolvedValue({ connectionId: 'B', reused: true })
    const { rerender } = await renderSwappable('A', /* attachOnly */ false)

    await rerender('B')

    expect(terminalDetachFn).not.toHaveBeenCalled()
    expect(resolveFn).not.toHaveBeenCalled()
  })

  it('StrictMode: a fresh mount fires NO swap — the identity guard holds under double-invoked effects', async () => {
    resolveFn.mockResolvedValue({ connectionId: 'A', reused: true })
    useTerminalStore.setState({
      sessions: new Map([['A', { id: 'A', connectionId: 'A' }]]),
    } as never)

    await act(async () => {
      render(
        createElement(
          StrictMode,
          null,
          createElement(XtermTerminal, {
            sessionId: 'A',
            isActive: true,
            isVisible: true,
            attachOnly: true,
          }),
        ),
      )
    })

    // The swap keys off the sessionId CHANGING from what is attached; a mount — even
    // one whose effects React deliberately double-invokes — has changed nothing, so
    // it never RE-RESOLVES a connection. (StrictMode's simulated unmount does run the
    // detach-on-unmount effect once; crowbar-bridge's connection-identity guard is
    // what keeps that from tearing down the real re-mounted socket.)
    expect(resolveFn).not.toHaveBeenCalled()
  })

  it('hands the connection hook the new connectionId with a bumped reconnectKey (the re-listen trigger)', async () => {
    resolveFn.mockResolvedValue({ connectionId: 'B', reused: true })
    const { rerender } = await renderSwappable('A')
    expect(connectionCalls.at(-1)).toEqual({ connectionId: 'A', reconnectKey: 0 })

    await rerender('B')

    // useTerminalConnection re-runs its listen effect on [connectionId, reconnectKey]:
    // the new id re-targets terminalListen, and the bump guarantees re-registration
    // even when the id string were unchanged. (That a bump actually re-registers the
    // listener is proven at hook level — use-terminal-connection.test.ts drives a
    // reconnectKey rerender through the real effect.)
    expect(connectionCalls.at(-1)).toEqual({ connectionId: 'B', reconnectKey: 1 })
  })

  it('re-injects the terminal link styles for the incoming session after a swap', async () => {
    resolveFn.mockResolvedValue({ connectionId: 'B', reused: true })
    const { rerender } = await renderSwappable('A')

    await rerender('B')

    // The init effect's sessionId-keyed cleanup removed the OLD session's styles...
    expect(removeLinkStylesFn).toHaveBeenCalledWith('A')
    // ...and the swap re-injects for the NEW one, scoped to the container's new id.
    // initializeTerminal — the only other injector — never runs again, because the
    // whole point is that the component does not remount.
    expect(injectLinkStylesFn).toHaveBeenCalledWith('B', 'terminal-B')
  })
})

// RACING SWAPS MUST RECONCILE, NOT DROP. doReconnect serializes on the init lock
// (isInitializingRef) and silently returns when it is held — right for a transport
// drop storm, fatal for a swap chain: back-to-back runner replacements (A→B→C while
// B's resolve is in flight) used to lose C entirely, leaving the connection effect
// listening on a connectionId whose transport nobody ever opened (a permanently
// blank pane) and the intermediate B transport attached with nobody to release it
// (its detach cleanup had already run, before the transport existed). The lock
// holder now reconciles on release: if the pane re-pointed the terminal while it was
// resolving, it releases the orphaned intermediate transport and fires the swap
// again — each pass attaches the LATEST desired session, so the chain converges.
describe('XtermTerminal attach-only swap reconciliation (racing swaps)', () => {
  it("A→B→C with B's resolve in flight: converges on C — resolves B then C, releases B, never touches C's transport", async () => {
    const b = deferred<{ connectionId: string; reused: boolean }>()
    resolveFn.mockImplementationOnce(() => b.promise)
    resolveFn.mockImplementation((args: unknown) => {
      const { tabSessionId } = args as { tabSessionId: string }
      return Promise.resolve({ connectionId: tabSessionId, reused: true })
    })
    const { result, rerender, onSessionGone } = await renderSwappable('A')
    const node = result.container.querySelector('.xterm-container')

    await rerender('B') // the B swap starts; the test holds its resolve in flight
    expect(resolveFn).toHaveBeenCalledTimes(1)

    await rerender('C') // lands while B is resolving — must be DEFERRED, not dropped
    // The in-flight resolve holds the lock, so no second resolve has started yet.
    expect(resolveFn).toHaveBeenCalledTimes(1)

    await act(async () => {
      b.resolve({ connectionId: 'B', reused: true })
    })
    await act(async () => {})

    // The completing B-swap reconciled against the latest desired session and
    // attached C. Exactly two resolves, in order — the dropped C swap was re-fired
    // once, not doubled by the effect AND the reconcile.
    expect(resolveFn).toHaveBeenCalledTimes(2)
    const tabIds = resolveFn.mock.calls.map((c) => (c[0] as { tabSessionId: string }).tabSessionId)
    expect(tabIds).toEqual(['B', 'C'])
    // A was closed on the way out; B — whose transport landed AFTER its own detach
    // cleanup had run — was released by the reconcile; C's transport is never touched.
    const detached = terminalDetachFn.mock.calls.map((c) => c[0])
    expect(detached).toContain('A')
    expect(detached).toContain('B')
    expect(detached).not.toContain('C')
    expect(terminalCreateFn).not.toHaveBeenCalled()
    expect(onSessionGone).not.toHaveBeenCalled()
    // The connection hook ends on C, reconnectKey bumped once per landed attach.
    expect(connectionCalls.at(-1)).toEqual({ connectionId: 'C', reconnectKey: 2 })
    // The store points C at its resolved connection, and it is still the SAME
    // mounted component throughout the whole chain.
    expect(useTerminalStore.getState().getSession('C')?.connectionId).toBe('C')
    expect(result.container.querySelector('.xterm-container')).toBe(node)
  })

  it('a swap that resolves GONE mid-chain still reconciles forward to the latest session', async () => {
    // B's PTY died before its swap landed (CLI crashed on startup). The gone signal
    // fires — the pane will decide what to render — but the terminal must STILL move
    // on to C, which arrived while B was resolving: gone-for-B is not gone-for-C.
    const b = deferred<{ gone: true }>()
    resolveFn.mockImplementationOnce(() => b.promise)
    resolveFn.mockImplementation((args: unknown) => {
      const { tabSessionId } = args as { tabSessionId: string }
      return Promise.resolve({ connectionId: tabSessionId, reused: true })
    })
    const { rerender, onSessionGone } = await renderSwappable('A')

    await rerender('B')
    await rerender('C')
    await act(async () => {
      b.resolve({ gone: true })
    })
    await act(async () => {})

    expect(onSessionGone).toHaveBeenCalledTimes(1) // B's death was reported...
    const tabIds = resolveFn.mock.calls.map((c) => (c[0] as { tabSessionId: string }).tabSessionId)
    expect(tabIds).toEqual(['B', 'C']) // ...and C was still attached afterwards
    expect(connectionCalls.at(-1)).toEqual({ connectionId: 'C', reconnectKey: 1 })
    expect(terminalCreateFn).not.toHaveBeenCalled()
  })
})
