import { createElement } from 'react'
import { act, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// FINDING 2 — hidden→visible in an UNFOCUSED pane must re-push the PTY size.
//
// The fitTerminal path suppresses the daemon PTY push while an attach-only view is
// HIDDEN (a background chat tab fits xterm locally but routes the resize into a
// no-op), delegating the re-push to a become-visible fit. The active-catch-up that
// performed that re-push is gated on pane FOCUS (isActive = isActivePane &&
// isVisible), so a chat revealed in an UNFOCUSED pane — its sibling tab closed while
// its split pane is not focused — after a geometry change while hidden was left with
// a stale daemon PTY until the pane was focused (a transient misdraw). The fix adds a
// size-reconcile gated on VISIBILITY alone (attachOnly only). These tests assert that
// reconcile fires on show without pane focus, and that shell tabs are untouched.
//
// The reconcile effect is gated on isInitialized && xtermRef.current, so — unlike the
// other terminal component suites, which leave xterm uninitialized behind a 0×0 rect
// — this harness must drive initializeTerminal to completion. It fakes @xterm/xterm
// and the addons, gives the container a real box, and flushes rAF deterministically.
// The observable reconcile is refitAndSyncPty (fit + PTY push, mocked to a spy).

const { refitSpy, terminalResizeFn, terminalDetachFn, stableConnRef, writeBufferedFn, resolveFn } =
  vi.hoisted(() => ({
    refitSpy: vi.fn(),
    terminalResizeFn: vi.fn(async (..._a: unknown[]) => {}),
    terminalDetachFn: vi.fn(async (..._a: unknown[]) => {}),
    // STABLE across renders so fitTerminal keeps its identity — otherwise every
    // rerender re-runs the theme/font effects, which would call fitTerminal and
    // muddy the "did the show transition reconcile?" assertion.
    stableConnRef: { current: 'C' as string | null },
    writeBufferedFn: vi.fn(),
    resolveFn: vi.fn(async (..._a: unknown[]) => ({ connectionId: 'C', reused: true })),
  }))

// A minimal xterm stand-in: initializeTerminal only reads/writes these members, and
// faking it keeps the real (canvas/WebGL) terminal out of jsdom.
vi.mock('@xterm/xterm', () => {
  class FakeTerminal {
    rows = 24
    cols = 80
    options: Record<string, unknown> = {}
    unicode = { activeVersion: '6' }
    textarea: HTMLTextAreaElement | null = null
    constructor(_opts: unknown) {}
    open() {}
    focus() {}
    blur() {}
    clear() {}
    selectAll() {}
    clearSelection() {}
    getSelection() {
      return ''
    }
    paste() {}
    scrollToTop() {}
    scrollToBottom() {}
    refresh() {}
    dispose() {}
    attachCustomKeyEventHandler() {}
    loadAddon() {}
    registerLinkProvider() {
      return { dispose() {} }
    }
  }
  return { Terminal: FakeTerminal }
})

vi.mock('@/features/terminal/hooks/use-terminal-addons', () => ({
  createTerminalAddons: () => ({
    fitAddon: { fit: () => {}, proposeDimensions: () => ({ cols: 80, rows: 24 }) },
    searchAddon: {
      onDidChangeResults: () => ({ dispose: () => {} }),
      clearDecorations: () => {},
      findNext: () => false,
      findPrevious: () => false,
    },
    serializeAddon: { serialize: () => '' },
    webglAddon: null,
  }),
  injectLinkStyles: vi.fn(),
  loadWebLinksAddon: vi.fn(),
  removeLinkStyles: vi.fn(),
}))

// The reconcile under test = refitAndSyncPty (fit + PTY push). Spy on it and assert
// it is invoked on the become-visible transition. pollUntilResizeSettles is only
// reached from the (inert) ResizeObserver here, so a no-op canceller suffices.
vi.mock('@/features/terminal/lib/refit', () => ({
  refitAndSyncPty: (...a: unknown[]) => refitSpy(...a),
  pollUntilResizeSettles: () => () => {},
}))

vi.mock('@/features/terminal/utils/resolve-font', () => ({
  resolveTerminalFont: async () => ({ fontFamily: 'monospace', skipWebGL: true }),
}))

// getTerminalTheme MUST be a stable reference so the theme effect does not re-run on
// the visibility rerenders (a re-run would call fitTerminal and confound the spy).
vi.mock('@/features/terminal/hooks/use-terminal-theme', () => {
  const getTerminalTheme = () => ({})
  return { useTerminalTheme: () => ({ getTerminalTheme }) }
})

vi.mock('@/features/terminal/lib/terminal-file-links', () => ({
  registerTerminalFileLinks: vi.fn(),
  workspaceRelativePath: vi.fn(() => null),
}))

vi.mock('@/features/terminal/hooks/use-terminal-connection', () => ({
  useTerminalConnection: () => ({
    currentConnectionIdRef: stableConnRef,
    writeBuffered: writeBufferedFn,
  }),
}))

vi.mock('@/features/terminal/components/resolve-terminal-connection', () => ({
  resolveTerminalConnection: (...a: unknown[]) => resolveFn(...a),
}))

vi.mock('@/lib/crowbar-bridge', () => ({
  terminalCreate: vi.fn(async () => 'C'),
  terminalDetach: (...a: unknown[]) => terminalDetachFn(...a),
  terminalListLive: vi.fn(async () => [] as string[]),
  terminalResize: (...a: unknown[]) => terminalResizeFn(...a),
  onTransportDrop: () => () => {},
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

import { XtermTerminal } from '@/features/terminal/components/terminal'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'

const SESSION = 'agent-term'

// Deterministic requestAnimationFrame: callbacks queue and only run when the test
// flushes them, so init's async chain and every fitTerminal pass advance in lockstep
// under our control instead of on jsdom's ~16ms real timer.
let rafMap = new Map<number, FrameRequestCallback>()
let rafSeq = 0

async function flushFrames(rounds = 20) {
  for (let i = 0; i < rounds; i++) {
    const batch = [...rafMap.values()]
    rafMap = new Map()
    for (const cb of batch) cb(i * 16)
    // Let init's awaited promise chain (font resolve, connection resolve) advance
    // between frames so isInitialized flips and the trailing fit rAF gets scheduled.
    await Promise.resolve()
    await Promise.resolve()
  }
}

// A fit scheduled by a STATE update (e.g. setIsInitialized) has its passive effect —
// and therefore its runFit rAF — flushed on the act BOUNDARY, after a single
// flushFrames has already run. Cycle act+flush a few times so those effects run AND
// their frames drain, leaving no init-era rAF to leak into the step under test.
async function settle() {
  for (let i = 0; i < 3; i++) {
    await act(async () => {
      await flushFrames()
    })
  }
}

const REAL_RECT = { width: 400, height: 300, top: 0, left: 0, right: 400, bottom: 300, x: 0, y: 0 }

beforeEach(() => {
  refitSpy.mockClear()
  terminalResizeFn.mockClear()
  terminalDetachFn.mockClear()
  writeBufferedFn.mockClear()
  resolveFn.mockClear()
  resolveFn.mockResolvedValue({ connectionId: 'C', reused: true })
  stableConnRef.current = 'C'
  rafMap = new Map()
  rafSeq = 0
  useTerminalStore.setState({ sessions: new Map() } as never)

  vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => {
    rafSeq += 1
    rafMap.set(rafSeq, cb)
    return rafSeq
  })
  vi.stubGlobal('cancelAnimationFrame', (id: number) => {
    rafMap.delete(id)
  })
  // jsdom has no layout — give every element a real box so the init/fit 0-area
  // guards pass and initializeTerminal proceeds.
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue(REAL_RECT as DOMRect)
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

const el = (props: { attachOnly: boolean; isActive: boolean; isVisible: boolean }) =>
  createElement(XtermTerminal, {
    sessionId: SESSION,
    workspaceId: 'w1',
    isActive: props.isActive,
    isVisible: props.isVisible,
    attachOnly: props.attachOnly,
  })

/** Render an already-initialized terminal (init driven to completion), visible. */
async function renderInitialized(attachOnly: boolean) {
  useTerminalStore.setState({
    sessions: new Map([[SESSION, { id: SESSION, connectionId: 'C' }]]),
  } as never)

  let result!: ReturnType<typeof render>
  await act(async () => {
    result = render(el({ attachOnly, isActive: false, isVisible: true }))
  })
  await settle()
  return result
}

async function rerender(
  result: ReturnType<typeof render>,
  props: { attachOnly: boolean; isActive: boolean; isVisible: boolean },
) {
  await act(async () => {
    result.rerender(el(props))
  })
  await settle()
}

describe('XtermTerminal become-visible PTY size reconcile', () => {
  it('reconciles size when an attach-only view becomes visible in an UNFOCUSED pane', async () => {
    const result = await renderInitialized(/* attachOnly */ true)
    // Sanity: init completed and the reconcile path is wired (init fit ran).
    expect(refitSpy).toHaveBeenCalled()
    refitSpy.mockClear()

    // Hide it (its geometry may change while hidden; the PTY push is suppressed)...
    await rerender(result, { attachOnly: true, isActive: false, isVisible: false })
    // ...then reveal it WITHOUT focusing its pane (isActive stays false).
    await rerender(result, { attachOnly: true, isActive: false, isVisible: true })

    // The size-reconcile fired on visibility alone — no pane focus required. Pre-fix
    // the only catch-up was focus-gated (isActive=false), so nothing reconciled and
    // the daemon PTY stayed stale until the pane was focused.
    expect(refitSpy).toHaveBeenCalled()
  })

  it('still reconciles on show when the pane IS focused (focus path intact)', async () => {
    const result = await renderInitialized(/* attachOnly */ true)
    refitSpy.mockClear()

    await rerender(result, { attachOnly: true, isActive: false, isVisible: false })
    await rerender(result, { attachOnly: true, isActive: true, isVisible: true })

    expect(refitSpy).toHaveBeenCalled()
  })

  it('NO REGRESSION: a shell tab becoming visible while unfocused runs no new reconcile', async () => {
    // Shell tabs push their PTY size even while hidden, so they are never stale on
    // show and keep only the focus-gated catch-up. The new visibility reconcile is
    // attachOnly-gated and must not fire for them — visible-but-unfocused stays inert.
    const result = await renderInitialized(/* attachOnly */ false)
    refitSpy.mockClear()

    await rerender(result, { attachOnly: false, isActive: false, isVisible: false })
    await rerender(result, { attachOnly: false, isActive: false, isVisible: true })

    expect(refitSpy).not.toHaveBeenCalled()
  })
})
