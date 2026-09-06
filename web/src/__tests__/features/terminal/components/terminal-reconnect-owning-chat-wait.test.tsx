import { createElement } from 'react'
import { act, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// BUG C — doReconnect built its chat-scoped base URL (terminalsBaseForWorkspace)
// BEFORE its own try block even started, and EVERY caller invokes it as
// `void doReconnect()` with no `.catch` (onTransportDrop here). A throw from
// terminalsBaseForWorkspace — the workspace's owning chat not recorded yet — was
// therefore an UNHANDLED PROMISE REJECTION, and worse: `isInitializingRef.current`
// had already been set to `true` moments later in the ORIGINAL flow with nothing
// left alive to release it, since the throw skipped straight past the try/finally
// that would have called releaseInitLock(). The init lock stayed held forever,
// blocking every future init/reconnect attempt on that terminal.
//
// The fix waits for the owning chat at the very top of doReconnect, before any of
// that runs. This test drives a transport drop while the workspace has NO
// recorded scope (the throw trigger) and asserts: (1) nothing downstream fires
// (no resolveTerminalConnection call — the gate returned cleanly, no exception
// propagates out of the void'd promise), and (2) the terminal is NOT stuck: once
// the owning chat is recorded and another drop lands, reconnection proceeds
// normally — proving the lock was never left held.
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

vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getActiveWorkspaceId: () => 'w-target',
}))

// NOT mocked: the REAL chat-scoped URL builder + workspace-scope registry run,
// so the missing-owning-chat throw this test exercises is the genuine one.

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
import { recordWorkspaceScope, __resetWorkspaceScopesForTest } from '@/lib/workspace-scope'

const SESSION = 'already-attached-term'

async function fireLatestDrop() {
  const cb = dropCallbacks.at(-1)
  await act(async () => {
    cb?.()
  })
  await act(async () => {})
}

// void doReconnect() (every real caller: onTransportDrop, the imperative-reattach
// swap effect) attaches no `.catch` — a throw from terminalsBaseForWorkspace
// before the fix surfaces as a genuine Node `unhandledRejection`, which Vitest
// itself would flag as a run-level error even though the synchronous assertions
// in the test body still pass. Capturing it explicitly makes that failure mode
// an assertion in THIS test rather than something only visible in the run's exit
// code.
let unhandled: unknown
const onUnhandledRejection = (reason: unknown) => {
  unhandled = reason
}

beforeEach(() => {
  resolveFn.mockReset()
  saveReconnectFn.mockClear()
  dropCallbacks.length = 0
  unhandled = undefined
  process.on('unhandledRejection', onUnhandledRejection)
  useTerminalStore.setState({
    sessions: new Map([[SESSION, { id: SESSION, connectionId: SESSION }]]),
  } as never)
  // No recorded scope at all for w-target — the sidebar hasn't caught up yet.
  __resetWorkspaceScopesForTest()
})

afterEach(() => {
  process.off('unhandledRejection', onUnhandledRejection)
})

describe('XtermTerminal.doReconnect — waits for the owning chat instead of throwing', () => {
  it('a transport drop with no recorded owning chat does not call resolveTerminalConnection (no unhandled throw), and reconnects cleanly once the chat is recorded', async () => {
    await act(async () => {
      render(
        createElement(XtermTerminal, {
          sessionId: SESSION,
          workspaceId: 'w-target',
          isActive: false,
          isVisible: false,
        }),
      )
    })

    // Drop while unready: the pre-fix code threw here, before the try/catch and
    // before releaseInitLock could ever run — this call must be a clean no-op.
    await fireLatestDrop()
    expect(resolveFn).not.toHaveBeenCalled()
    expect(unhandled).toBeUndefined()

    // The sidebar catches up.
    resolveFn.mockResolvedValue({ connectionId: 'reattached', reused: true })
    await act(async () => {
      recordWorkspaceScope({
        projectId: 'p1',
        repoId: 'r1',
        wsId: 'w-target',
        owningChatId: 'chat-target',
      })
    })

    // A second drop (the effect re-subscribed on the fresh, now-ready doReconnect)
    // must succeed normally — proving the earlier bail never left the init lock
    // stuck, and never crashed anything downstream of it.
    await fireLatestDrop()
    expect(resolveFn).toHaveBeenCalled()
    const args = resolveFn.mock.calls.at(-1)?.[0] as { base: string }
    expect(args.base).toBe('/v0/chats/chat-target/terminals')
    expect(saveReconnectFn).toHaveBeenCalledWith('w-target', SESSION, 'reattached')
  })
})
