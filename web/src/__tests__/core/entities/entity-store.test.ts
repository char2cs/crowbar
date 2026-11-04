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
import { followChatLifecycle } from '@/features/agent/stores/chat-lifecycle'
import { chatStore } from '@/features/agent/stores/chat-store'
import type { OutboxItem } from '@/features/agent/stores/lib/outbox'
import { chatEntity, workspaceDTO } from '@/__tests__/__fixtures__/sidebar-entities'

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
  release = holdScope('local', 'projects')
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
    synced()
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

  it("holds a snapshot's rows back until its synced, then lands them together", () => {
    reconnect(`e${epochs}`)
    entity('op1', 2)
    entity('op2', 2)
    expect(held('op1')).toBeUndefined()
    synced()
    expect(held('op1')).toBeDefined()
    expect(held('op2')).toBeDefined()
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
    const stop = holdScope('local', 'ws:w1')
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

  it('places over a held row at its version, so only a newer frame replaces it', () => {
    folderFrame('f1', 2, 0)
    placeEntities('folder', { f1: folder('f1', 7) })
    expect(folders().f1).toMatchObject({ v: 2, data: { order: 7 } })
    folderFrame('f1', 1, 9)
    expect(folders().f1?.data.order).toBe(7)
    folderFrame('f1', 3, 8)
    expect(folders().f1?.data.order).toBe(8)
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

describe('a projects snapshot that could not read a repo', () => {
  const row = (kind: string, id: string, data: unknown) =>
    socket.deliver({ t: 'entity', scope: 'projects', kind, id, v: 2, data })
  const repo = (id: string) => row('repo', id, { id, projectId: 'p1', name: id, order: 0 })
  const rowsOf = (repoId: string) => {
    row('workspace', `w-${repoId}`, workspaceDTO({ id: `w-${repoId}`, repoId }))
    row(
      'chat',
      `c-${repoId}`,
      chatEntity({ id: `c-${repoId}`, repoId, workspaceId: `w-${repoId}` }),
    )
    row('folder', `f-${repoId}`, {
      id: `f-${repoId}`,
      projectId: 'p1',
      repoId,
      parentId: '',
      name: 'F',
      order: 0,
    })
  }
  const byKind = () => entityStore.getState().byKind
  const kept = (repoId: string) => [
    `w-${repoId}` in byKind().workspace,
    `c-${repoId}` in byKind().chat,
    `f-${repoId}` in byKind().folder,
  ]
  let stopLifecycle = () => {}
  beforeEach(() => {
    vi.spyOn(console, 'warn').mockImplementation(() => {})
    repo('r1')
    repo('r2')
    rowsOf('r1')
    rowsOf('r2')
    stopLifecycle = followChatLifecycle()
  })
  afterEach(() => {
    stopLifecycle()
    vi.restoreAllMocks()
  })

  it("keeps the rows of a repo it could not read, and prunes a read repo's", () => {
    reconnect(`e${epochs}`)
    repo('r1')
    socket.deliver({ t: 'synced', scope: 'projects', unread: ['r1'] })
    expect(kept('r1')).toEqual([true, true, true])
    expect(kept('r2')).toEqual([false, false, false])
    expect(console.warn).toHaveBeenCalledOnce()
  })

  it("keeps the home rows of a project it could not read, and prunes a read project's", () => {
    rowsOf('')
    row('chat', 'c-p2', chatEntity({ id: 'c-p2', projectId: 'p2' }))
    reconnect(`e${epochs}`)
    socket.deliver({ t: 'synced', scope: 'projects', unread: ['p1'] })
    expect(kept('')).toEqual([true, true, true])
    expect('c-p2' in byKind().chat).toBe(false)
  })

  it('still lets the repo list itself follow the snapshot', () => {
    reconnect(`e${epochs}`)
    socket.deliver({ t: 'synced', scope: 'projects', unread: ['r2'] })
    expect(Object.keys(byKind().repo)).toEqual([])
    expect(kept('r2')).toEqual([true, true, true])
  })

  it('keeps a held chat its prompts waiting to be sent, and prunes it once a snapshot reads it', () => {
    const waiting = [{ requestId: 'q1' } as OutboxItem]
    chatStore.setState({ outbox: { 'c-r1': waiting } })
    reconnect(`e${epochs}`)
    socket.deliver({ t: 'synced', scope: 'projects', unread: ['r1'] })
    expect(chatStore.getState().outbox['c-r1']).toBe(waiting)

    reconnect(`e${epochs}`)
    synced()
    expect(chatStore.getState().outbox['c-r1']).toBeUndefined()
  })
})
