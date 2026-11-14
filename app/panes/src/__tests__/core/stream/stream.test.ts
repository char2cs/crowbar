import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import * as reporting from '@/lib/errors/report'
import { entityStore } from '@/core/entities/entity-store'
import {
  holding,
  holdScope,
  onSignal,
  registerScopeKind,
  startStream,
  watchScope,
} from '@/core/stream/stream'
import type { Frame } from '@/core/stream/frames'
import * as localDaemon from '@/features/hosts/lib/local-daemon'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { pairRemoteHost } from '@/__tests__/__fixtures__/remote-host'

interface FakeSocket {
  sent: unknown[]
  deliver: (data: unknown) => void
  drop: () => void
  subscribed: boolean
}

const sockets = vi.hoisted(() => {
  const all = new Map<string, FakeSocket>()
  const of = (endpoint: string): FakeSocket => {
    let s = all.get(endpoint)
    if (!s) {
      s = { sent: [], deliver: () => {}, drop: () => {}, subscribed: false }
      all.set(endpoint, s)
    }
    return s
  }
  return { all, of }
})

vi.mock('@/lib/ws/manager', () => ({
  wsManager: {
    subscribe: (endpoint: string, cb: (data: unknown) => void) => {
      const s = sockets.of(endpoint)
      s.deliver = cb
      s.subscribed = true
      return () => {
        s.subscribed = false
        s.deliver = () => {}
      }
    },
    send: (endpoint: string, data: unknown) => void sockets.of(endpoint).sent.push(data),
    onDrop: (endpoint: string, listener: () => void) => {
      sockets.of(endpoint).drop = listener
      return () => {}
    },
  },
}))

const L = 'local'
const socket = sockets.of('/v0/stream')
const host1 = sockets.of('crowbar://host-1/v0/stream')

let epochs = 0
const cleanups: (() => void)[] = []
const hold = (scope: string, hostId = L) => {
  const release = holdScope(hostId, scope)
  cleanups.push(release)
  return release
}
const register = (...args: Parameters<typeof registerScopeKind>) =>
  cleanups.push(registerScopeKind(...args))
/** A fresh socket for each test: a drop, then the daemon's ready. */
const reconnect = (epoch = `e${epochs}`) => {
  socket.drop()
  socket.deliver({ t: 'ready', epoch })
}
const op = (id: string, v: number) => ({
  t: 'entity',
  scope: 'projects',
  kind: 'operation',
  id,
  v,
  data: { id, kind: 'git.push', subject: { kind: 'workspace', id: 'w1' }, items: [] },
})

beforeAll(() => {
  startStream()
})

/** What went out besides the projects scope, which the stream itself holds from boot. */
const sentBesidesProjects = () =>
  socket.sent.filter((m) => (m as { scope?: string }).scope !== 'projects')

beforeEach(() => {
  epochs++
  reconnect()
  socket.sent.length = 0
})

afterEach(() => {
  cleanups.splice(0).forEach((stop) => stop())
  vi.restoreAllMocks()
})

describe('the one stream', () => {
  it('sends sub on the first hold of a scope and unsub on the last release', () => {
    const first = hold('chat:a')
    const second = hold('chat:a')
    first()
    expect(socket.sent).toEqual([{ op: 'sub', scope: 'chat:a' }])
    first()
    second()
    expect(socket.sent).toEqual([
      { op: 'sub', scope: 'chat:a' },
      { op: 'unsub', scope: 'chat:a' },
    ])
  })

  it('holds a scope taken while the socket is down until the next ready', () => {
    socket.drop()
    hold('chat:a')
    expect(socket.sent).toEqual([])
    socket.deliver({ t: 'ready', epoch: `e${epochs}` })
    expect(sentBesidesProjects()).toEqual([{ op: 'sub', scope: 'chat:a' }])
  })

  it('re-sends sub for every held scope after a reconnect, with since from the kind handler', () => {
    register('chat', { since: (_h, key) => (key === 'a' ? 42 : 0), frame: () => {} })
    hold('chat:a')
    hold('ws:w1')
    socket.sent.length = 0

    reconnect()

    expect(sentBesidesProjects()).toEqual([
      { op: 'sub', scope: 'chat:a', since: 42 },
      { op: 'sub', scope: 'ws:w1' },
    ])
  })

  it('a closed resync frame resubscribes that scope', () => {
    const frames: Frame[] = []
    register('chat', { frame: (_h, f) => void frames.push(f) })
    hold('chat:a')
    socket.sent.length = 0

    socket.deliver({ t: 'closed', scope: 'chat:a', reason: 'resync' })

    expect(frames).toEqual([{ t: 'closed', scope: 'chat:a', reason: 'resync' }])
    expect(socket.sent).toEqual([{ op: 'sub', scope: 'chat:a' }])
  })

  it('a closed refused frame reports its problem once and drops the scope', () => {
    const report = vi.spyOn(reporting, 'report').mockImplementation(() => {})
    const release = hold('chat:a')
    socket.deliver({
      t: 'closed',
      scope: 'chat:a',
      reason: 'refused',
      problem: { code: 'not_found', message: 'no such chat' },
    })
    expect(report).toHaveBeenCalledTimes(1)
    expect((report.mock.calls[0][0] as Error).message).toBe('no such chat')

    socket.sent.length = 0
    reconnect()
    release()
    expect(sentBesidesProjects()).toEqual([])
  })

  it('an undecodable frame is reported once and its scope dropped', () => {
    const report = vi.spyOn(reporting, 'report').mockImplementation(() => {})
    hold('chat:a')
    socket.deliver({ t: 'synced', scope: 'chat:a', surprise: true })
    socket.deliver({ t: 'synced', scope: 'chat:a', surprise: true })
    expect(report).toHaveBeenCalledTimes(1)

    socket.sent.length = 0
    reconnect()
    expect(sentBesidesProjects()).toEqual([])
  })

  it('an undecodable projects frame is reported and the projects asked for again, never let go', () => {
    const report = vi.spyOn(reporting, 'report').mockImplementation(() => {})
    socket.deliver({ t: 'synced', scope: 'projects' })
    socket.deliver({ t: 'synced', scope: 'projects', surprise: true })
    expect(report).toHaveBeenCalledTimes(1)
    expect(socket.sent).toEqual([
      { op: 'unsub', scope: 'projects' },
      { op: 'sub', scope: 'projects' },
    ])
    expect(holding(L, 'projects')).toBe(true)
  })

  it('a snapshot that keeps failing tells its watch how often, until one lands', () => {
    const failed = vi.fn()
    cleanups.push(watchScope(L, 'projects', { failed }))
    socket.deliver({ t: 'synced', scope: 'projects' })
    reconnect()
    expect(failed).not.toHaveBeenCalled()
    reconnect()
    expect(failed).toHaveBeenLastCalledWith(1)
    reconnect()
    expect(failed).toHaveBeenLastCalledWith(2)
    socket.deliver({ t: 'synced', scope: 'projects' })
    reconnect()
    reconnect()
    expect(failed).toHaveBeenLastCalledWith(1)
  })

  it('an error frame is reported and drops no scope', () => {
    const report = vi.spyOn(reporting, 'report').mockImplementation(() => {})
    hold('chat:a')
    socket.deliver({
      t: 'error',
      problem: { code: 'invalid_argument', message: 'stream: unknown op' },
    })
    expect(report).toHaveBeenCalledTimes(1)

    socket.sent.length = 0
    reconnect()
    expect(sentBesidesProjects()).toEqual([{ op: 'sub', scope: 'chat:a' }])
  })

  it('holds the projects scope from boot, across every reconnect', () => {
    reconnect()
    expect(socket.sent).toEqual([{ op: 'sub', scope: 'projects' }])
  })

  // A snapshot the daemon could not read closes the socket with resync, never the scope.
  it('a socket closed for resync resubscribes every held scope on its next ready', () => {
    hold('projects')
    socket.sent.length = 0

    reconnect()

    expect(socket.sent).toEqual([{ op: 'sub', scope: 'projects' }])
  })

  it('a ready frame with a new epoch reaches every registered kind before any entity frame', () => {
    const calls: string[] = []
    register('chat', { epoch: (_h, e) => void calls.push(`chat ${e}`), frame: () => {} })
    register('lsp', {
      epoch: (_h, e) => void calls.push(`lsp ${e}`),
      frame: (_h, f) => void calls.push(`lsp ${f.t}`),
    })
    hold('lsp:c1')

    reconnect('restarted')
    socket.deliver({ t: 'synced', scope: 'lsp:c1' })
    reconnect('restarted')

    expect(calls).toEqual([
      `chat e${epochs}`,
      `lsp e${epochs}`,
      'chat restarted',
      'lsp restarted',
      'lsp synced',
    ])
  })

  // A lazily loaded feature registers after the first ready; it must still
  // know which daemon run its frames come from.
  it('a kind registered after ready learns the current epoch at once', () => {
    reconnect('current')
    const heard: string[] = []
    register('chat', { epoch: (_h, e) => void heard.push(e), frame: () => {} })
    expect(heard).toEqual(['current'])
    reconnect('current')
    reconnect('next')
    expect(heard).toEqual(['current', 'next'])
  })

  it('a resubscribe snapshot removes an entity deleted while offline', () => {
    hold('projects')
    socket.deliver(op('op1', 2))
    socket.deliver(op('op2', 2))
    socket.deliver({ t: 'synced', scope: 'projects' })
    expect(Object.keys(entityStore.getState().byKind.operation)).toEqual(
      expect.arrayContaining(['op1', 'op2']),
    )

    reconnect()
    socket.deliver(op('op1', 5))
    socket.deliver({ t: 'synced', scope: 'projects' })

    expect(entityStore.getState().byKind.operation.op1?.v).toBe(5)
    expect(entityStore.getState().byKind.operation.op2).toBeUndefined()
  })

  it('onSignal delivers only its scope and kind', () => {
    const heard: unknown[] = []
    cleanups.push(onSignal(L, 'git:w1', 'git_status', (data) => void heard.push(data)))
    hold('git:w1')
    hold('git:w2')

    socket.deliver({ t: 'signal', scope: 'git:w1', kind: 'git_status', data: { n: 1 } })
    socket.deliver({ t: 'signal', scope: 'git:w2', kind: 'git_status', data: { n: 2 } })
    socket.deliver({ t: 'signal', scope: 'git:w1', kind: 'file_change', data: { n: 3 } })

    expect(heard).toEqual([{ n: 1 }])
  })

  it('a scope watch hears every synced of its scope, the resubscribe one too', () => {
    const synced = vi.fn()
    cleanups.push(watchScope(L, 'files:w1', { synced }))
    hold('files:w1')
    socket.deliver({ t: 'synced', scope: 'files:w1' })
    socket.deliver({ t: 'synced', scope: 'files:w2' })
    reconnect()
    socket.deliver({ t: 'synced', scope: 'files:w1' })

    expect(synced).toHaveBeenCalledTimes(2)
  })

  it('a refusal a scope watch hears is its to tell, not reported, and the scope is let go', () => {
    const report = vi.spyOn(reporting, 'report').mockImplementation(() => {})
    const refused = vi.fn()
    cleanups.push(watchScope(L, 'doc:/tmp/a', { refused }))
    hold('doc:/tmp/a')
    const problem = { code: 'watch_limit', message: 'no watches left' }

    socket.deliver({ t: 'closed', scope: 'doc:/tmp/a', reason: 'refused', problem })

    expect(refused).toHaveBeenCalledWith(problem)
    expect(report).not.toHaveBeenCalled()
    expect(holding(L, 'doc:/tmp/a')).toBe(false)
  })

  it('a watch on a scope already synced hears it at once; one on a resubscribing scope waits', () => {
    hold('doc:/tmp/a')
    socket.deliver({ t: 'synced', scope: 'doc:/tmp/a' })
    const late = vi.fn()
    cleanups.push(watchScope(L, 'doc:/tmp/a', { synced: late }))
    expect(late).toHaveBeenCalledTimes(1)

    socket.drop()
    const waiting = vi.fn()
    cleanups.push(watchScope(L, 'doc:/tmp/a', { synced: waiting }))
    expect(waiting).not.toHaveBeenCalled()
    socket.deliver({ t: 'ready', epoch: `e${epochs}` })
    socket.deliver({ t: 'synced', scope: 'doc:/tmp/a' })
    socket.deliver({ t: 'closed', scope: 'doc:/tmp/a', reason: 'resync' })
    waiting.mockClear()
    cleanups.push(watchScope(L, 'doc:/tmp/a', { synced: waiting }))
    expect(waiting).not.toHaveBeenCalled()
  })

  it('a scope let go or refused is not synced to the next watch on it', () => {
    vi.spyOn(reporting, 'report').mockImplementation(() => {})
    const release = hold('doc:/tmp/b')
    socket.deliver({ t: 'synced', scope: 'doc:/tmp/b' })
    release()
    hold('doc:/tmp/c')
    socket.deliver({ t: 'synced', scope: 'doc:/tmp/c' })
    socket.deliver({ t: 'closed', scope: 'doc:/tmp/c', reason: 'refused' })

    const synced = vi.fn()
    cleanups.push(watchScope(L, 'doc:/tmp/b', { synced }), watchScope(L, 'doc:/tmp/c', { synced }))
    expect(synced).not.toHaveBeenCalled()
  })
})

describe('one stream per host', () => {
  /** host-1 paired, its socket open and ready, nothing it sent kept. */
  const pairHost1 = () => {
    pairRemoteHost('host-1')
    host1.deliver({ t: 'ready', epoch: 'h1' })
    host1.sent.length = 0
  }

  afterEach(() => {
    useHostConfigStore.getState().removeHost('host-1')
    host1.sent.length = 0
  })

  it('a scope held on host-1 is subscribed on crowbar://host-1/v0/stream, bare', () => {
    pairHost1()
    hold('ws:w1', 'host-1')

    expect(host1.sent).toEqual([{ op: 'sub', scope: 'ws:w1' }])
    expect(sentBesidesProjects()).toEqual([])
  })

  it('a paired host holds its own projects scope from its first ready', () => {
    pairRemoteHost('host-1')
    expect(host1.subscribed).toBe(true)
    host1.deliver({ t: 'ready', epoch: 'h1' })
    expect(host1.sent).toEqual([{ op: 'sub', scope: 'projects' }])
  })

  it('a frame from host-1 reaches the kind handler with host-1', () => {
    const heard: [string, Frame][] = []
    register('lsp', { frame: (hostId, f) => void heard.push([hostId, f]) })
    pairHost1()
    hold('lsp:c1', 'host-1')
    hold('lsp:c1')

    host1.deliver({ t: 'synced', scope: 'lsp:c1' })

    expect(heard).toEqual([['host-1', { t: 'synced', scope: 'lsp:c1' }]])
  })

  it('a drop of host-1 leaves local scopes synced', () => {
    pairHost1()
    hold('files:w1')
    socket.deliver({ t: 'synced', scope: 'files:w1' })

    host1.drop()
    const synced = vi.fn()
    cleanups.push(watchScope(L, 'files:w1', { synced }))

    expect(synced).toHaveBeenCalledTimes(1)
  })

  it("host-1's ready resubscribes only host-1's holds", () => {
    pairHost1()
    hold('ws:w1', 'host-1')
    hold('ws:w2')
    host1.sent.length = 0
    socket.sent.length = 0

    host1.drop()
    host1.deliver({ t: 'ready', epoch: 'h1' })

    expect(host1.sent).toEqual([
      { op: 'sub', scope: 'projects' },
      { op: 'sub', scope: 'ws:w1' },
    ])
    expect(socket.sent).toEqual([])
  })

  it('a hold on a host whose socket has not said ready goes out when it does', () => {
    pairRemoteHost('host-1')
    hold('git:w1', 'host-1')
    expect(host1.sent).toEqual([])

    host1.deliver({ t: 'ready', epoch: 'h1' })

    expect(host1.sent).toContainEqual({ op: 'sub', scope: 'git:w1' })
  })

  it('removing host-1 stops its socket and tells every handler to forget it', () => {
    const forgot: string[] = []
    register('lsp', { frame: () => {}, forget: (hostId) => void forgot.push(hostId) })
    pairHost1()
    hold('lsp:c1', 'host-1')

    useHostConfigStore.getState().removeHost('host-1')

    expect(host1.subscribed).toBe(false)
    expect(forgot).toEqual(['host-1'])
    expect(holding('host-1', 'lsp:c1')).toBe(false)
  })

  it('a second start returns the running stream', () => {
    expect(startStream()).toBe(startStream())
  })

  it('no local socket opens when the install has no local daemon', () => {
    startStream()()
    vi.spyOn(localDaemon, 'hasLocalDaemon').mockReturnValue(false)
    try {
      startStream()
      expect(socket.subscribed).toBe(false)
      pairRemoteHost('host-1')
      expect(host1.subscribed).toBe(true)
    } finally {
      startStream()()
      vi.mocked(localDaemon.hasLocalDaemon).mockRestore()
      startStream()
    }
  })
})
