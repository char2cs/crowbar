import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import * as reporting from '@/lib/errors/report'
import { entityStore } from '@/core/entities/entity-store'
import { holdScope, registerScopeKind, startStream } from '@/core/stream/stream'
import type { Frame } from '@/core/stream/frames'

const socket = vi.hoisted(() => ({
  sent: [] as unknown[],
  deliver: (_data: unknown) => {},
  drop: () => {},
}))

vi.mock('@/lib/ws/manager', () => ({
  wsManager: {
    subscribe: (_endpoint: string, cb: (data: unknown) => void) => {
      socket.deliver = cb
      return () => {}
    },
    send: (_endpoint: string, data: unknown) => void socket.sent.push(data),
    onDrop: (_endpoint: string, listener: () => void) => {
      socket.drop = listener
      return () => {}
    },
  },
}))

let epochs = 0
const cleanups: (() => void)[] = []
const hold = (scope: string) => {
  const release = holdScope(scope)
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
    expect(socket.sent).toEqual([{ op: 'sub', scope: 'chat:a' }])
  })

  it('re-sends sub for every held scope after a reconnect, with since from the kind handler', () => {
    register('chat', { since: (key) => (key === 'a' ? 42 : 0), frame: () => {} })
    hold('chat:a')
    hold('ws:w1')
    socket.sent.length = 0

    reconnect()

    expect(socket.sent).toEqual([
      { op: 'sub', scope: 'chat:a', since: 42 },
      { op: 'sub', scope: 'ws:w1' },
    ])
  })

  it('a closed resync frame resubscribes that scope', () => {
    const frames: Frame[] = []
    register('chat', { frame: (f) => void frames.push(f) })
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
    expect(socket.sent).toEqual([])
  })

  it('an undecodable frame is reported once and its scope dropped', () => {
    const report = vi.spyOn(reporting, 'report').mockImplementation(() => {})
    hold('chat:a')
    socket.deliver({ t: 'synced', scope: 'chat:a', surprise: true })
    socket.deliver({ t: 'synced', scope: 'chat:a', surprise: true })
    expect(report).toHaveBeenCalledTimes(1)

    socket.sent.length = 0
    reconnect()
    expect(socket.sent).toEqual([])
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
    expect(socket.sent).toEqual([{ op: 'sub', scope: 'chat:a' }])
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
    register('chat', { epoch: (e) => void calls.push(`chat ${e}`), frame: () => {} })
    register('ws', {
      epoch: (e) => void calls.push(`ws ${e}`),
      frame: (f) => void calls.push(`ws ${f.t}`),
    })
    hold('ws:w1')

    reconnect('restarted')
    socket.deliver({ t: 'synced', scope: 'ws:w1' })
    reconnect('restarted')

    expect(calls).toEqual(['chat restarted', 'ws restarted', 'ws synced'])
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
})
