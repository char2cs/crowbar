import { createElement } from 'react'
import { act, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// BUG B — initializeTerminal fires ~200ms after a terminal becomes visible
// (including on tab restore during workspace activation), and used to build its
// chat-scoped base URL (terminalsBaseForWorkspace) UNCONDITIONALLY. The sidebar
// records a workspace's owning chat id asynchronously — independent of, and often
// slower than, the workspace's own hydration — so a terminal that activates first
// found no owning chat recorded yet and terminalsBaseForWorkspace THREW. That
// throw was caught by initializeTerminal's own try/catch, but `xtermRef.current`
// had already been assigned earlier in the SAME function, before the throw point.
// The retry effect gates re-entry on `xtermRef.current` being null, so once it was
// set the terminal never tried again: permanently blank. The fix waits for the
// owning chat BEFORE touching xtermRef at all, and the retry effect's own
// chatScopeReady dependency re-fires it the moment the id arrives.
//
// This harness drives a REAL initializeTerminal to completion (same technique as
// terminal-visible-reconcile.test.tsx): fake @xterm/xterm + addons, deterministic
// rAF, and the REAL chat-scoped URL builder (workspace-scope.ts is not mocked) so
// the missing-owning-chat throw is the genuine one, not a stand-in.

const { terminalCreateFn, resolveFn } = vi.hoisted(() => ({
  terminalCreateFn: vi.fn(async (..._a: unknown[]) => 'fresh-conn'),
  resolveFn: vi.fn(async (..._a: unknown[]) => ({ connectionId: 'fresh-conn', reused: false })),
}))

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

vi.mock('@/features/terminal/lib/refit', () => ({
  refitAndSyncPty: vi.fn(),
  pollUntilResizeSettles: () => () => {},
}))

vi.mock('@/features/terminal/utils/resolve-font', () => ({
  resolveTerminalFont: async () => ({ fontFamily: 'monospace', skipWebGL: true }),
}))

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
    currentConnectionIdRef: { current: null },
    writeBuffered: vi.fn(),
  }),
}))

vi.mock('@/features/terminal/components/resolve-terminal-connection', () => ({
  resolveTerminalConnection: (...a: unknown[]) => resolveFn(...a),
}))

vi.mock('@/lib/crowbar-bridge', () => ({
  terminalCreate: (...a: unknown[]) => terminalCreateFn(...a),
  terminalDetach: vi.fn(async () => {}),
  terminalListLive: vi.fn(async () => [] as string[]),
  terminalResize: vi.fn(async () => {}),
  onTransportDrop: () => () => {},
}))

vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getActiveWorkspaceId: () => 'w-unready',
}))

vi.mock('@/features/terminal/lib/terminal-reconnect-map', () => ({
  saveReconnect: vi.fn(),
  loadReconnect: vi.fn(() => null),
  clearReconnect: vi.fn(),
}))

// NOT mocked: the REAL chat-scoped URL builder AND the REAL workspace-scope
// registry run, so terminalsBaseForWorkspace genuinely throws until
// recordWorkspaceScope below supplies an owningChatId — the actual bug trigger.
import { XtermTerminal } from '@/features/terminal/components/terminal'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'
import { recordWorkspaceScope, __resetWorkspaceScopesForTest } from '@/lib/workspace-scope'

const SESSION = 'shell-term'

let rafMap = new Map<number, FrameRequestCallback>()
let rafSeq = 0

async function flushFrames(rounds = 20) {
  for (let i = 0; i < rounds; i++) {
    const batch = [...rafMap.values()]
    rafMap = new Map()
    for (const cb of batch) cb(i * 16)
    await Promise.resolve()
    await Promise.resolve()
  }
}

async function settle() {
  for (let i = 0; i < 3; i++) {
    await act(async () => {
      await flushFrames()
    })
  }
}

const REAL_RECT = { width: 400, height: 300, top: 0, left: 0, right: 400, bottom: 300, x: 0, y: 0 }

beforeEach(() => {
  terminalCreateFn.mockClear()
  resolveFn.mockClear()
  resolveFn.mockResolvedValue({ connectionId: 'fresh-conn', reused: false })
  rafMap = new Map()
  rafSeq = 0
  useTerminalStore.setState({ sessions: new Map() } as never)
  // The workspace under test has NO recorded scope at all — the sidebar's
  // chat-list fetch hasn't landed yet, matching the real race this fix closes.
  __resetWorkspaceScopesForTest()

  vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => {
    rafSeq += 1
    rafMap.set(rafSeq, cb)
    return rafSeq
  })
  vi.stubGlobal('cancelAnimationFrame', (id: number) => {
    rafMap.delete(id)
  })
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue(REAL_RECT as DOMRect)
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('XtermTerminal — waits for the owning chat before initializing', () => {
  it('does not spawn a PTY while the workspace has no recorded owning chat, then initializes once it arrives', async () => {
    // A pre-existing connectionId makes initializeTerminal skip its real 100ms
    // font-settle `setTimeout` (hadConnectionAtInitStart) — irrelevant to what
    // this test asserts, and avoids needing fake timers alongside the
    // deterministic rAF stand-in above.
    useTerminalStore.setState({
      sessions: new Map([[SESSION, { id: SESSION, connectionId: 'preexisting' }]]),
    } as never)

    let result!: ReturnType<typeof render>
    await act(async () => {
      result = render(
        createElement(XtermTerminal, {
          sessionId: SESSION,
          workspaceId: 'w-unready',
          isActive: false,
          isVisible: true,
        }),
      )
    })
    await settle()

    // No owning chat recorded yet: terminalsBaseForWorkspace would throw, and the
    // fix bails BEFORE ever reaching resolveTerminalConnection/terminalCreate.
    expect(resolveFn).not.toHaveBeenCalled()
    expect(terminalCreateFn).not.toHaveBeenCalled()

    // The sidebar's chat-list fetch lands, recording the owning chat.
    await act(async () => {
      recordWorkspaceScope({
        projectId: 'p1',
        repoId: 'r1',
        wsId: 'w-unready',
        owningChatId: 'chat-unready',
      })
    })
    await settle()

    // PRE-FIX: xtermRef.current was already assigned before the throw, so the
    // retry effect (gated on xtermRef.current being null) never tried again and
    // this assertion would fail — the terminal stayed permanently blank.
    expect(resolveFn).toHaveBeenCalled()
    const args = resolveFn.mock.calls.at(-1)?.[0] as { base: string }
    expect(args.base).toBe('/v0/chats/chat-unready/terminals')

    result.unmount()
  })
})
