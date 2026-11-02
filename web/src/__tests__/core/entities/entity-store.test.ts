import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  entityStore,
  forgetEntities,
  patchEntities,
  placeEntities,
} from '@/core/entities/entity-store'
import { holdScope, startStream } from '@/core/stream/stream'
import { useLocalOperationsStore } from '@/lib/store/local-operations'
import { operationsState, putLocalOperation, type Operation } from '@/core/entities/operations'

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
  entityStore.setState(entityStore.getInitialState(), true)
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

  it('an older frame arriving after a newer daemon reply keeps the reply', () => {
    putLocalOperation({ ...operation('op1', 'failed'), dismissed: true, version: 4 })
    entity('op1', 2, { ...operation('op1', 'failed'), version: 3 })
    expect(operationsState().ops.op1).toMatchObject({ dismissed: true, version: 4 })

    entity('op1', 3, { ...operation('op1', 'failed'), dismissed: true, version: 4 })
    expect(useLocalOperationsStore.getState().ops).toEqual({})
  })

  it('synced forgets a daemon reply the stream no longer carries, never an in-flight one', () => {
    putLocalOperation({ ...operation('pruned'), version: 2 })
    putLocalOperation({ ...operation('mine'), local: true, version: 0 })
    reconnect(`e${epochs}`)
    synced()
    expect(Object.keys(useLocalOperationsStore.getState().ops)).toEqual(['mine'])
  })

  it('a ws scope carries review threads: snapshot, live change, gone, and pruning at synced', () => {
    const thread = (id: string, v: number, body: string, gone?: true) =>
      socket.deliver({
        t: 'entity',
        scope: 'ws:w1',
        kind: 'review_thread',
        id,
        v,
        ...(gone ? { gone } : { data: { id, workspaceId: 'w1', body } }),
      })
    const threads = () => entityStore.getState().byKind.review_thread
    const stop = holdScope('ws:w1')
    reconnect(`e${epochs}`)
    thread('t1', 2, 'first')
    thread('t2', 2, 'second')
    socket.deliver({ t: 'synced', scope: 'ws:w1' })
    thread('t1', 3, 'edited')
    expect(threads().t1?.data).toMatchObject({ body: 'edited' })

    thread('t2', 4, '', true)
    expect(threads().t2).toBeUndefined()

    reconnect(`e${epochs}`)
    socket.deliver({ t: 'synced', scope: 'ws:w1' })
    expect(threads().t1).toBeUndefined()
    stop()
  })

  it('the projects snapshot marks the store loaded, and a later resubscribe keeps it so', () => {
    expect(entityStore.getState().loaded.projects).toBe(true)
    reconnect(`e${epochs}`)
    expect(entityStore.getState().synced.projects).toBeUndefined()
    expect(entityStore.getState().loaded.projects).toBe(true)
  })
})

describe('a write answer applied before its frames', () => {
  const folder = (id: string, order: number) => ({
    id,
    projectId: 'p1',
    repoId: 'r1',
    parentId: '',
    name: id,
    order,
    createdAt: '2026-10-31T12:00:00Z',
  })
  const folderFrame = (id: string, v: number, order: number) =>
    socket.deliver({
      t: 'entity',
      scope: 'projects',
      kind: 'folder',
      id,
      v,
      data: folder(id, order),
    })
  const folders = () => entityStore.getState().byKind.folder

  it('places a row no frame carried yet, which its first frame then replaces', () => {
    placeEntities('folder', { f1: folder('f1', 3) })
    expect(folders().f1).toMatchObject({ v: 0, data: { order: 3 } })
    folderFrame('f1', 1, 4)
    expect(folders().f1).toMatchObject({ v: 1, data: { order: 4 } })
  })

  it('patches a held row at its version, and the revert yields to a newer frame', () => {
    folderFrame('f1', 2, 0)
    const revert = patchEntities('folder', { f1: { order: 5 } })
    expect(folders().f1).toMatchObject({ v: 2, data: { order: 5 } })
    revert()
    expect(folders().f1?.data.order).toBe(0)

    const second = patchEntities('folder', { f1: { order: 6 } })
    folderFrame('f1', 3, 6)
    second()
    expect(folders().f1).toMatchObject({ v: 3, data: { order: 6 } })
  })

  it('forgets rows a delete answered for, and leaves the store alone for one it never held', () => {
    folderFrame('f1', 1, 0)
    expect(Object.keys(folders())).toEqual(['f1'])
    const before = entityStore.getState()
    forgetEntities('folder', ['f-unknown'])
    expect(entityStore.getState()).toBe(before)
    forgetEntities('folder', ['f1'])
    expect(folders()).toEqual({})
  })
})
