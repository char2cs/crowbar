import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  entityOf,
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
import { chatEntity, projectDTO, workspaceDTO } from '@/__tests__/__fixtures__/sidebar-entities'
import { pairRemoteHost } from '@/__tests__/__fixtures__/remote-host'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'

interface FakeSocket {
  deliver: (data: unknown) => void
  drop: () => void
}
const sockets = vi.hoisted(() => {
  const by = new Map<string, FakeSocket>()
  return {
    of(endpoint: string): FakeSocket {
      let s = by.get(endpoint)
      if (!s) by.set(endpoint, (s = { deliver: () => {}, drop: () => {} }))
      return s
    },
  }
})
const LOCAL_ENDPOINT = '/v0/stream'
const HOST1_ENDPOINT = 'crowbar://host-1/v0/stream'
const socket = sockets.of(LOCAL_ENDPOINT)
const host1 = sockets.of(HOST1_ENDPOINT)

vi.mock('@/lib/ws/manager', () => ({
  wsManager: {
    subscribe: (endpoint: string, cb: (data: unknown) => void) => {
      sockets.of(endpoint).deliver = cb
      return () => {
        sockets.of(endpoint).deliver = () => {}
      }
    },
    send: () => {},
    onDrop: (endpoint: string, listener: () => void) => {
      sockets.of(endpoint).drop = listener
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
    expect(entityStore.getState().epochs.local).toBe(`e${epochs}-restarted`)
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

describe('two hosts in one store', () => {
  const project = (on: FakeSocket, id: string, v: number, over: object = {}) =>
    on.deliver({
      t: 'entity',
      scope: 'projects',
      kind: 'project',
      id,
      v,
      data: { ...projectDTO({ id }), ...over },
    })
  const projects = () => entityStore.getState().byKind.project
  const readyHost1 = (epoch = 'h1') => {
    host1.drop()
    host1.deliver({ t: 'ready', epoch })
    host1.deliver({ t: 'synced', scope: 'projects' })
  }

  beforeEach(() => {
    pairRemoteHost('host-1')
    readyHost1()
  })
  afterEach(() => {
    useHostConfigStore.getState().removeHost('host-1')
    useHostConfigStore.setState({ hosts: [], remoteEverPaired: false })
  })

  it("two hosts' rows with one raw id are both held", () => {
    project(socket, 'p1', 1, { name: 'here' })
    project(host1, 'p1', 1, { name: 'there' })
    expect(entityOf(entityStore.getState(), 'project', 'local', 'p1')?.name).toBe('here')
    expect(entityOf(entityStore.getState(), 'project', 'host-1', 'p1')?.name).toBe('there')
  })

  it('a payload naming another hostId is filed under the socket host', () => {
    project(host1, 'p9', 1, { hostId: 'local' })
    expect(entityOf(entityStore.getState(), 'project', 'local', 'p9')).toBeUndefined()
    expect(entityOf(entityStore.getState(), 'project', 'host-1', 'p9')?.hostId).toBe('host-1')
  })

  it("host-1's synced prunes none of local's rows", () => {
    project(socket, 'p1', 1)
    project(host1, 'p2', 1)
    readyHost1()
    expect(Object.keys(projects())).toEqual(['p1'])
    expect(entityStore.getState().synced['host-1~projects']).toBe(true)
    expect(entityStore.getState().synced.projects).toBe(true)
  })

  it("host-1's new epoch voids only host-1's versions", () => {
    project(socket, 'p1', 5, { name: 'local v5' })
    project(host1, 'p1', 5, { name: 'remote v5' })
    host1.drop()
    host1.deliver({ t: 'ready', epoch: 'h2' })
    project(host1, 'p1', 1, { name: 'remote v1' })
    host1.deliver({ t: 'synced', scope: 'projects' })
    project(socket, 'p1', 1, { name: 'local v1' })
    expect(entityOf(entityStore.getState(), 'project', 'host-1', 'p1')?.name).toBe('remote v1')
    expect(entityOf(entityStore.getState(), 'project', 'local', 'p1')?.name).toBe('local v5')
  })

  it("forgetHost drops that host's rows and scopes, nothing else", () => {
    project(socket, 'p1', 1)
    project(host1, 'p1', 1)
    useHostConfigStore.getState().removeHost('host-1')
    const s = entityStore.getState()
    expect(Object.keys(s.byKind.project)).toEqual(['p1'])
    expect(Object.keys(s.synced).filter((scope) => scope.startsWith('host-1'))).toEqual([])
    expect(s.epochs['host-1']).toBeUndefined()
    expect(s.synced.projects).toBe(true)
  })
})
