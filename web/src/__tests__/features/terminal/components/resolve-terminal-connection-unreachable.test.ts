/**
 * "I COULD NOT ASK" IS NOT "YOUR SESSION IS DEAD".
 *
 * resolveTerminalConnection decides whether a terminal tab re-attaches to its PTY,
 * spawns a new one, or reports the session gone. It answers that from the daemon's
 * live-session list — and that list used to be read as
 * `listLiveSessions().catch(() => [])`, so a request that merely FAILED became
 * "the daemon has no sessions", which every caller reads as "yours is dead".
 *
 * Under attachOnly (the agent chat pane, whose PTY is a vendor CLI) that resolved
 * to `{ gone: true }`, and the pane latched "This agent has exited" with a Resume
 * button over a CLI that was alive and working — while `clearReconnect` threw away
 * the one record that could have re-attached it. Nothing re-reads the chat
 * afterwards, so the pane stayed wrong, and its composer's `live` stayed false,
 * which freezes the prompt queue at "1 queued".
 *
 * This runs on mount AND on every transport-drop reconnect — and a reconnect is
 * precisely the moment the daemon is most likely to be briefly unreachable, which
 * is what made a "cannot happen" conflation an ordinary one. For a plain shell tab
 * the same conflation spawned a fresh PTY over a terminal that was still running.
 *
 * An EMPTY list is still a real answer and must still report gone — that is the
 * whole point of telling the two apart, so both directions are pinned here.
 */
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { terminalAttachFn, terminalHasTransportFn, loadReconnectFn, clearReconnectFn } = vi.hoisted(
  () => ({
    terminalAttachFn: vi.fn(async (..._a: unknown[]) => {}),
    terminalHasTransportFn: vi.fn((_id: string) => false),
    loadReconnectFn: vi.fn((..._a: unknown[]) => undefined as string | undefined),
    clearReconnectFn: vi.fn((..._a: unknown[]) => {}),
  }),
)

vi.mock('@/lib/crowbar-bridge', () => ({
  terminalAttach: (...a: unknown[]) => terminalAttachFn(...a),
  terminalHasTransport: (id: string) => terminalHasTransportFn(id),
}))

vi.mock('@/features/terminal/lib/terminal-reconnect-map', () => ({
  loadReconnect: (...a: unknown[]) => loadReconnectFn(...a),
  clearReconnect: (...a: unknown[]) => clearReconnectFn(...a),
}))

import { resolveTerminalConnection } from '@/features/terminal/components/resolve-terminal-connection'

/** The resolver retries an unhelpful answer once, behind a 400ms delay. Fake
 *  timers drive that deterministically — the assertions still block on the
 *  resolver's own promise, never on elapsed wall-clock time. */
async function resolveWithRetryElapsed<T>(run: () => Promise<T>): Promise<T> {
  const pending = run()
  await vi.advanceTimersByTimeAsync(400)
  return pending
}

const baseArgs = {
  workspaceId: 'w1',
  tabSessionId: 'tab-1',
  base: 'http://daemon',
  createTerminal: async () => 'fresh-pty',
}

beforeEach(() => {
  vi.useFakeTimers()
  terminalAttachFn.mockClear()
  terminalHasTransportFn.mockClear()
  loadReconnectFn.mockReset()
  loadReconnectFn.mockReturnValue(undefined)
  clearReconnectFn.mockClear()
})

describe('resolveTerminalConnection: an unreachable daemon', () => {
  it('TestRegression_AFailedLiveListIsNotAGoneSession', async () => {
    const createTerminal = vi.fn(async () => 'fresh-pty')
    const result = await resolveWithRetryElapsed(() =>
      resolveTerminalConnection({
        ...baseArgs,
        storeConnectionId: 'pty-alive',
        listLiveSessions: () => Promise.reject(new Error('daemon unreachable')),
        createTerminal,
        attachOnly: true,
      }),
    )

    // THE REGRESSION ASSERTION: a failed question must not be answered "dead".
    expect(result).toEqual({ unknown: true })
    expect('gone' in result).toBe(false)
    // ...and the one record that can re-attach the still-live PTY must survive.
    expect(clearReconnectFn).not.toHaveBeenCalled()
    expect(createTerminal).not.toHaveBeenCalled()
  })

  it('TestRegression_AFailedLiveListDoesNotSpawnOverALiveShellTab', async () => {
    const createTerminal = vi.fn(async () => 'fresh-pty')
    const result = await resolveWithRetryElapsed(() =>
      resolveTerminalConnection({
        ...baseArgs,
        storeConnectionId: 'pty-alive',
        listLiveSessions: () => Promise.reject(new Error('daemon unreachable')),
        createTerminal,
        // A plain shell tab: spawning is its normal fallback, which is exactly
        // why it must not be triggered by a question that was never answered.
        attachOnly: false,
      }),
    )

    expect(result).toEqual({ unknown: true })
    expect(createTerminal).not.toHaveBeenCalled()
  })

  it('keeps the persisted reconnect mapping when the daemon cannot be asked', async () => {
    loadReconnectFn.mockReturnValue('pty-persisted')
    const result = await resolveWithRetryElapsed(() =>
      resolveTerminalConnection({
        ...baseArgs,
        storeConnectionId: undefined,
        listLiveSessions: () => Promise.reject(new Error('daemon unreachable')),
        attachOnly: true,
      }),
    )

    expect(result).toEqual({ unknown: true })
    expect(clearReconnectFn).not.toHaveBeenCalled()
  })

  it('recovers when the retry succeeds: a first-attempt failure is not final', async () => {
    const listLiveSessions = vi
      .fn<() => Promise<string[]>>()
      .mockRejectedValueOnce(new Error('hiccup'))
      .mockResolvedValueOnce(['pty-alive'])

    const result = await resolveWithRetryElapsed(() =>
      resolveTerminalConnection({
        ...baseArgs,
        storeConnectionId: 'pty-alive',
        listLiveSessions,
        attachOnly: true,
      }),
    )

    expect(result).toEqual({ connectionId: 'pty-alive', reused: true })
    expect(terminalAttachFn).toHaveBeenCalledWith('pty-alive', 'http://daemon')
  })

  it('still reports gone for an AUTHORITATIVE empty list — the honest death path', async () => {
    const result = await resolveWithRetryElapsed(() =>
      resolveTerminalConnection({
        ...baseArgs,
        storeConnectionId: 'pty-dead',
        listLiveSessions: async () => [],
        attachOnly: true,
      }),
    )

    expect(result).toEqual({ gone: true })
    expect(clearReconnectFn).toHaveBeenCalledWith('w1', 'tab-1')
  })

  it('still spawns for a shell tab whose PTY is authoritatively absent', async () => {
    const createTerminal = vi.fn(async () => 'fresh-pty')
    const result = await resolveWithRetryElapsed(() =>
      resolveTerminalConnection({
        ...baseArgs,
        storeConnectionId: 'pty-dead',
        listLiveSessions: async () => ['someone-elses-pty'],
        createTerminal,
        attachOnly: false,
      }),
    )

    expect(result).toEqual({ connectionId: 'fresh-pty', reused: false })
    expect(createTerminal).toHaveBeenCalled()
  })
})
