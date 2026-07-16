import { createElement } from 'react'
import { act, render } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

// Workspace keep-alive keeps HIDDEN workspaces' terminals mounted and attached.
// The transport-drop reconnect used to resolve against getActiveWorkspaceId() —
// correct while only the active workspace could have mounted terminals, but with
// keep-alive a daemon restart drops EVERY mounted terminal's transport, and a
// hidden workspace A's terminal would then reconnect against active workspace B:
// createTerminal(B) spawns a shell in B's WORKTREE and rebinds A's tab, and the
// corruption persists via saveReconnect(B, A-session). These tests drive the real
// component's reconnect path and assert it resolves against the terminal's OWN
// workspace (the new workspaceId prop), not whichever workspace is active.
const { resolveFn, dropCallbacks, saveReconnectFn } = vi.hoisted(() => ({
  resolveFn: vi.fn(),
  dropCallbacks: [] as Array<() => void>,
  saveReconnectFn: vi.fn(),
}))

vi.mock('@/features/terminal/components/resolve-terminal-connection', () => ({
  resolveTerminalConnection: (...a: unknown[]) => resolveFn(...a),
}))

vi.mock('@/lib/crowbar-bridge', () => ({
  terminalCreate: vi.fn(async () => 'fresh-shell'),
  terminalDetach: vi.fn(async () => {}),
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

// The ACTIVE workspace is always w-active; the terminal under test belongs to
// w-hidden. Any resolve that reaches for w-active took the buggy path.
vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getActiveWorkspaceId: () => 'w-active',
}))

vi.mock('@/lib/workspace-scope-url', () => ({
  workspaceBase: (wsId: string) => `/v0/projects/p1/repos/r1/workspaces/${wsId}`,
}))

vi.mock('@/features/terminal/lib/terminal-reconnect-map', () => ({
  saveReconnect: (...a: unknown[]) => saveReconnectFn(...a),
  loadReconnect: vi.fn(() => null),
  clearReconnect: vi.fn(),
}))

vi.mock('@/features/terminal/hooks/use-terminal-connection', () => ({
  useTerminalConnection: () => ({
    currentConnectionIdRef: { current: null },
    writeBuffered: vi.fn(),
  }),
}))

vi.mock('@/features/terminal/hooks/use-terminal-addons', () => ({
  createTerminalAddons: vi.fn(),
  injectLinkStyles: vi.fn(),
  loadWebLinksAddon: vi.fn(),
  removeLinkStyles: vi.fn(),
}))

import { XtermTerminal } from '@/features/terminal/components/terminal'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'

const SESSION = 'hidden-ws-term'

async function renderAttached(props: { workspaceId?: string } = {}) {
  useTerminalStore.setState({
    sessions: new Map([[SESSION, { id: SESSION, connectionId: SESSION }]]),
  } as never)

  await act(async () => {
    render(
      createElement(XtermTerminal, {
        sessionId: SESSION,
        isActive: false,
        isVisible: false,
        ...props,
      }),
    )
  })
}

async function fireTransportDrop() {
  await act(async () => {
    for (const cb of [...dropCallbacks]) cb()
  })
  await act(async () => {})
}

beforeEach(() => {
  resolveFn.mockReset()
  saveReconnectFn.mockClear()
  dropCallbacks.length = 0
  useTerminalStore.setState({ sessions: new Map() } as never)
})

describe('XtermTerminal workspace targeting on reconnect', () => {
  it("a hidden workspace's terminal reconnects against ITS OWN workspace, not the active one", async () => {
    resolveFn.mockResolvedValue({ connectionId: 'reattached', reused: true })
    await renderAttached({ workspaceId: 'w-hidden' })

    await fireTransportDrop()

    expect(resolveFn).toHaveBeenCalled()
    const args = resolveFn.mock.calls.at(-1)?.[0] as { workspaceId: string; base: string }
    expect(args.workspaceId).toBe('w-hidden')
    expect(args.base).toContain('/workspaces/w-hidden/')
    // The reconnect mapping is persisted under the OWNING workspace too — a
    // wrong-workspace save here is what made the corruption survive restarts.
    expect(saveReconnectFn).toHaveBeenCalledWith('w-hidden', SESSION, 'reattached')
  })

  it('falls back to the active workspace when no workspaceId is threaded (legacy callers)', async () => {
    resolveFn.mockResolvedValue({ connectionId: 'reattached', reused: true })
    await renderAttached()

    await fireTransportDrop()

    const args = resolveFn.mock.calls.at(-1)?.[0] as { workspaceId: string }
    expect(args.workspaceId).toBe('w-active')
  })
})
