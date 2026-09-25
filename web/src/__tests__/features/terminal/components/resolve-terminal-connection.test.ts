import { beforeEach, describe, expect, it, vi } from 'vitest'
import { resolveTerminalSession } from '@/features/terminal/components/resolve-terminal-connection'
import { loadReconnect, saveReconnect } from '@/features/terminal/lib/terminal-reconnect-map'

// Which daemon PTY a terminal view attaches to. The invariants:
//   - a bound session the daemon still has is attached, never replaced;
//   - B7: a bound session the daemon no longer has ENDED — it is reported gone,
//     for a shell tab as much as an agent view, and never replaced by a spawn;
//   - an agent view is bound to the PTY it names, and never spawns;
//   - only a tab that was never bound gets a fresh PTY;
//   - a daemon that could not be asked is not a death: nothing changes.

const createTerminal = vi.fn(async () => 'fresh-pty')
const listLive = vi.fn(async () => ['pty-1'])

function resolve(args: {
  storeConnectionId?: string
  attachOnly?: boolean
  listLiveSessions?: () => Promise<string[]>
}) {
  return resolveTerminalSession({
    workspaceId: 'ws-1',
    tabSessionId: 'tab-1',
    storeConnectionId: args.storeConnectionId,
    listLiveSessions: args.listLiveSessions ?? listLive,
    createTerminal,
    attachOnly: args.attachOnly,
  })
}

beforeEach(() => {
  localStorage.clear()
  createTerminal.mockClear()
  listLive.mockReset()
  listLive.mockResolvedValue(['pty-1'])
})

describe('resolveTerminalSession', () => {
  it('attaches the in-memory bound session when the daemon has it', async () => {
    await expect(resolve({ storeConnectionId: 'pty-1' })).resolves.toEqual({
      sessionId: 'pty-1',
      created: false,
    })
    expect(createTerminal).not.toHaveBeenCalled()
  })

  it('attaches the persisted bound session after a reload', async () => {
    saveReconnect('ws-1', 'tab-1', 'pty-1')
    await expect(resolve({})).resolves.toEqual({ sessionId: 'pty-1', created: false })
    expect(createTerminal).not.toHaveBeenCalled()
  })

  it.each([false, true])(
    'B7: a bound session the daemon no longer has is gone, never replaced (attachOnly=%s)',
    async (attachOnly) => {
      saveReconnect('ws-1', 'tab-1', 'pty-dead')
      listLive.mockResolvedValue(['someone-elses-pty'])
      await expect(resolve({ storeConnectionId: 'pty-dead', attachOnly })).resolves.toEqual({
        gone: true,
      })
      expect(createTerminal).not.toHaveBeenCalled()
      expect(loadReconnect('ws-1', 'tab-1')).toBeNull()
    },
  )

  it('an empty list is an answer: one question, no timed retry', async () => {
    listLive.mockResolvedValue([])
    await expect(resolve({ storeConnectionId: 'pty-1' })).resolves.toEqual({ gone: true })
    expect(listLive).toHaveBeenCalledTimes(1)
  })

  it('gives a never-bound shell tab its first PTY, without asking the daemon for a list', async () => {
    await expect(resolve({})).resolves.toEqual({ sessionId: 'fresh-pty', created: true })
    expect(listLive).not.toHaveBeenCalled()
  })

  it('binds an agent view to the PTY it names, with no seeded mapping, and never spawns', async () => {
    listLive.mockResolvedValue(['tab-1'])
    await expect(resolve({ attachOnly: true })).resolves.toEqual({
      sessionId: 'tab-1',
      created: false,
    })
    listLive.mockResolvedValue([])
    await expect(resolve({ attachOnly: true })).resolves.toEqual({ gone: true })
    expect(createTerminal).not.toHaveBeenCalled()
  })

  it('a daemon that could not be asked changes nothing: unknown, mapping kept, no spawn', async () => {
    saveReconnect('ws-1', 'tab-1', 'pty-1')
    await expect(
      resolve({ listLiveSessions: () => Promise.reject(new Error('daemon unreachable')) }),
    ).resolves.toEqual({ unknown: true })
    expect(loadReconnect('ws-1', 'tab-1')).toBe('pty-1')
    expect(createTerminal).not.toHaveBeenCalled()
  })
})
