import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { entityStore } from '@/core/entities/entity-store'
import { holdScope, startStream } from '@/core/stream/stream'
import { useLocalOperationsStore } from '@/lib/store/local-operations'
import type { Operation } from '@/lib/store/operations'

const socket = vi.hoisted(() => ({ deliver: (_data: unknown) => {}, drop: () => {} }))

vi.mock('@/lib/ws/manager', () => ({
  wsManager: {
    subscribe: (_endpoint: string, cb: (data: unknown) => void) => {
      socket.deliver = cb
      return () => {}
    },
    send: () => {},
    onDrop: (_endpoint: string, listener: () => void) => {
      socket.drop = listener
      return () => {}
    },
  },
}))

function operation(id: string, state: Operation['state'] = 'running'): Operation {
  return {
    id,
    kind: 'git.push',
    subject: { kind: 'workspace', id: 'w1' },
    projectId: 'p1',
    label: 'Push',
    state,
    items: [],
    dismissed: false,
    version: 1,
    createdAt: '2026-10-31T12:00:00Z',
    updatedAt: '2026-10-31T12:00:00Z',
  }
}

const entity = (id: string, v: number, data: unknown = operation(id)) =>
  socket.deliver({ t: 'entity', scope: 'projects', kind: 'operation', id, v, data })
const synced = () => socket.deliver({ t: 'synced', scope: 'projects' })
const held = (id: string) => entityStore.getState().byKind.operation[id]

let epochs = 0
/** A new socket: its ready re-sends the projects sub, so a snapshot begins. */
const reconnect = (epoch: string) => {
  socket.drop()
  socket.deliver({ t: 'ready', epoch })
}

let release = () => {}
beforeAll(() => {
  startStream()
})
beforeEach(() => {
  epochs++
  entityStore.setState({ byKind: { operation: {} }, members: {}, fresh: {}, synced: {} })
  useLocalOperationsStore.setState({ ops: {} })
  release = holdScope('projects')
  reconnect(`e${epochs}`)
  synced()
})
afterEach(() => release())

describe('entity store', () => {
  it('applies a newer version and ignores an older one', () => {
    entity('op1', 3, operation('op1', 'failed'))
    entity('op1', 2, operation('op1', 'running'))
    expect(held('op1')).toMatchObject({ v: 3, data: { state: 'failed' } })

    entity('op1', 4, operation('op1', 'succeeded'))
    expect(held('op1')).toMatchObject({ v: 4, data: { state: 'succeeded' } })
  })

  it('an equal version keeps the stored object identity', () => {
    entity('op1', 3)
    const before = held('op1')
    entity('op1', 3)
    expect(held('op1')).toBe(before)
  })

  it('a gone frame removes the entity', () => {
    entity('op1', 3)
    socket.deliver({
      t: 'entity',
      scope: 'projects',
      kind: 'operation',
      id: 'op1',
      v: 4,
      gone: true,
    })
    expect(held('op1')).toBeUndefined()
  })

  it('a new epoch replaces every entity even at a lower version', () => {
    entity('op1', 900, operation('op1', 'running'))
    reconnect(`e${epochs}-restarted`)
    entity('op1', 1, operation('op1', 'failed'))
    expect(held('op1')).toMatchObject({ v: 1, data: { state: 'failed' } })
    expect(entityStore.getState().epoch).toBe(`e${epochs}-restarted`)
  })

  it('synced prunes the members of a scope not seen since its sub', () => {
    entity('op1', 2)
    entity('op2', 2)
    reconnect(`e${epochs}`)
    entity('op1', 5)
    synced()
    expect(held('op1')).toBeDefined()
    expect(held('op2')).toBeUndefined()
  })

  it('marks an operation met before its scope synced as a snapshot row', () => {
    reconnect(`e${epochs}`)
    entity('op1', 2)
    synced()
    entity('op2', 3)
    expect(held('op1')?.data.snapshot).toBe(true)
    expect(held('op2')?.data.snapshot).toBeUndefined()
  })

  it('an operation frame forgets the local operation with its id', () => {
    useLocalOperationsStore.setState({
      ops: {
        op1: { ...operation('op1'), local: true },
        mine: { ...operation('mine'), local: true },
      },
    })
    entity('op1', 2)
    expect(Object.keys(useLocalOperationsStore.getState().ops)).toEqual(['mine'])
  })
})
