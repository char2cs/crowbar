// The `/v0/projects` socket answers every subscribe with a snapshot — one
// complete ProjectDTO frame per project — and then pushes one complete DTO per
// change. Those frames ARE the list: merging them costs no request, so a boot
// makes exactly one GET /v0/projects however the snapshot and the seed GET
// interleave.
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { IDBFactory } from 'fake-indexeddb'
import { resetDB } from '@/lib/persistence/idb'
import { dataOf, failed, loading, success, type Loadable } from '@/lib/loadable'
import type { Project } from '@/lib/types'
import type { HostStatus } from '@/features/hosts/types'

// The transport builds its URLs through host resolution, which reads this store: if the
// store (or host resolution) loaded either module, that would be an import cycle again.
vi.mock('@/lib/api', () => {
  throw new Error('the project list store loaded api.ts')
})
vi.mock('@/lib/ws/manager', () => {
  throw new Error('the project list store loaded the socket manager')
})

// The list's transport, faked: each host's read and its `/v0/projects` socket.
const readLocal = vi.fn<() => Promise<Project[]>>()
const readRemote = vi.fn<(hostId: string) => Promise<Project[]>>()
const sockets = new Map<string, (frame: unknown) => void>()
const socketLog: string[] = []
const socket = {
  get emit() {
    return sockets.get('local') ?? null
  },
}
const transport = {
  read: (hostId: string) => (hostId === 'local' ? readLocal() : readRemote(hostId)),
  subscribe: (hostId: string, cb: (frame: unknown) => void) => {
    sockets.set(hostId, cb)
    socketLog.push(`open ${hostId}`)
    return () => {
      sockets.delete(hostId)
      socketLog.push(`close ${hostId}`)
    }
  },
}

/** A ProjectDTO frame as the daemon sends it — the wire names no host. */
const wire = (id: string, order: number, name = id.toUpperCase()) => ({
  id,
  name,
  path: `/${id}`,
  order,
  lastActivity: '2026-09-24T00:00:00Z',
})

/** The same project as the list read answers it: tagged with its host. */
const p = (id: string, order: number, name = id.toUpperCase()) =>
  ({ ...wire(id, order, name), hostId: 'local' }) as unknown as Project

/** The next GET parks until released; resolves once the request is sent. */
async function parkedRequest(): Promise<(rows: Project[]) => void> {
  let release!: (rows: Project[]) => void
  const answer = new Promise<Project[]>((resolve) => {
    release = resolve
  })
  readLocal.mockImplementationOnce(() => answer)
  return release
}

async function sent(times: number): Promise<void> {
  await vi.waitFor(() => expect(readLocal).toHaveBeenCalledTimes(times))
}

async function settle(): Promise<void> {
  // Past DELTA_DEBOUNCE_MS: any re-read a frame scheduled would have been sent.
  await vi.advanceTimersByTimeAsync(500)
}

async function freshStore() {
  vi.resetModules()
  const { connectProjectList, useProjectDataStore } = await import('@/lib/store/projects')
  connectProjectList(transport)
  return useProjectDataStore
}

beforeEach(() => {
  // A fresh database per test: a fresh store falls back on the cache the last one wrote.
  globalThis.indexedDB = new IDBFactory()
  resetDB()
  readLocal.mockReset()
  readRemote.mockReset()
  sockets.clear()
  socketLog.length = 0
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
})
afterEach(() => {
  vi.useRealTimers()
})

describe('useProjectDataStore', () => {
  it('loads, with host resolution, without the transport', async () => {
    vi.resetModules()
    await expect(import('@/lib/host-resolve')).resolves.toHaveProperty('hostIdForProject')
  })

  it('fetch populates loadable success', async () => {
    const store = await freshStore()
    readLocal.mockResolvedValueOnce([p('p1', 0)])
    await store.getState().fetch()
    expect(store.getState().data.status).toBe('success')
  })
})

describe('project list at boot', () => {
  it('a snapshot landing while the seed GET is in flight costs no second request', async () => {
    const store = await freshStore()
    const release = await parkedRequest()
    const done = store.getState().fetch()
    store.getState().startSync()
    await sent(1)
    socket.emit!(wire('a', 0))
    socket.emit!(wire('b', 1))
    release([p('a', 0), p('b', 1)])
    await done
    await settle()
    expect(readLocal).toHaveBeenCalledTimes(1)
    expect(dataOf(store.getState().data)?.map((x) => x.id)).toEqual(['a', 'b'])
  })

  it('a snapshot landing after the seed GET costs no second request and no re-render', async () => {
    const store = await freshStore()
    readLocal.mockResolvedValueOnce([p('a', 0), p('b', 1)])
    await store.getState().fetch()
    const seeded = store.getState().data
    store.getState().startSync()
    socket.emit!(wire('a', 0))
    socket.emit!(wire('b', 1))
    await settle()
    expect(readLocal).toHaveBeenCalledTimes(1)
    expect(store.getState().data).toBe(seeded)
  })

  it('a change that lands while the GET is in flight survives the older answer', async () => {
    const store = await freshStore()
    const release = await parkedRequest()
    const done = store.getState().fetch()
    store.getState().startSync()
    await sent(1)
    socket.emit!(wire('a', 0, 'Renamed'))
    release([p('a', 0, 'Old')])
    await done
    await settle()
    expect(dataOf(store.getState().data)?.[0].name).toBe('Renamed')
    expect(readLocal).toHaveBeenCalledTimes(1)
  })

  it('live frames upsert, reorder and tombstone without a request', async () => {
    const store = await freshStore()
    readLocal.mockResolvedValueOnce([p('a', 0), p('b', 1)])
    await store.getState().fetch()
    store.getState().startSync()
    socket.emit!(wire('c', 2))
    socket.emit!(wire('b', 0))
    socket.emit!(wire('a', 1))
    socket.emit!({ id: 'c', status: 'deleted' })
    await settle()
    expect(readLocal).toHaveBeenCalledTimes(1)
    expect(dataOf(store.getState().data)?.map((x) => x.id)).toEqual(['b', 'a'])
    expect(store.getState().data.status).toBe('success')
  })

  // The only socket feeding this list is the local daemon's; a frame merged
  // untagged would leave a project with no host for any caller to route by.
  it('a frame off the project socket is tagged with the local host it came from', async () => {
    const store = await freshStore()
    readLocal.mockResolvedValueOnce([p('a', 0)])
    await store.getState().fetch()
    store.getState().startSync()
    socket.emit!(wire('b', 1))
    socket.emit!(wire('a', 0, 'Renamed'))
    await settle()
    const held = dataOf(store.getState().data)
    expect(held?.map((x) => [x.id, x.hostId])).toEqual([
      ['a', 'local'],
      ['b', 'local'],
    ])
  })

  it('a reconnect re-reads the list (frames missed during the outage are unknowable)', async () => {
    const store = await freshStore()
    readLocal.mockResolvedValue([p('a', 0)])
    await store.getState().fetch()
    store.getState().startSync()
    socket.emit!({ reconnected: true })
    await settle()
    await vi.waitFor(() => expect(readLocal).toHaveBeenCalledTimes(2))
  })
})

describe('a paired host coming up', () => {
  const host = (id: string) => ({
    id,
    name: id,
    endpoint: `${id}.lan:3737`,
    pairedFingerprint: 'ff',
    clientId: `client-${id}`,
  })
  const remoteRow = (id: string, hostId: string) =>
    ({ ...wire(id, 0), hostId }) as unknown as Project

  /** A live list holding the local answer, with `hostIds` configured. */
  async function liveList(...hostIds: string[]) {
    const store = await freshStore()
    const { useHostConfigStore } = await import('@/features/hosts/stores/host-config-store')
    const { getOrCreateHostRuntime } = await import('@/features/hosts/stores/host-runtime-registry')
    useHostConfigStore.setState({ hosts: hostIds.map(host) })
    readLocal.mockResolvedValue([p('a', 0)])
    const stop = store.getState().startSync()
    await store.getState().fetch()
    return {
      store,
      stop,
      config: useHostConfigStore,
      status: (hostId: string, status: HostStatus) =>
        getOrCreateHostRuntime(hostId).getState().setStatus(status),
      held: () => dataOf(store.getState().data)?.map((x) => `${x.id}@${x.hostId}`),
    }
  }

  /** The next read of a remote host waits until it is answered with these rows. */
  function parkedRemote(): (rows: Project[]) => void {
    let answer!: (rows: Project[]) => void
    readRemote.mockImplementationOnce(
      () =>
        new Promise<Project[]>((resolve) => {
          answer = resolve
        }),
    )
    return (rows) => answer(rows)
  }

  // Boot reads the list while every host is still unknown: a host's rows arrive when it does.
  it('has its rows read and merged while the list is live, and no other host read', async () => {
    const list = await liveList('h1', 'h2')
    readRemote.mockImplementation(async (hostId) => [remoteRow(`r-${hostId}`, hostId)])

    list.status('h1', 'up')

    await vi.waitFor(() => expect(list.held()).toEqual(['a@local', 'r-h1@h1']))
    expect(readRemote.mock.calls).toEqual([['h1']])
    expect(readLocal).toHaveBeenCalledTimes(1)

    list.stop()
    list.status('h2', 'up')
    await settle()
    expect(readRemote.mock.calls).toEqual([['h1']])
  })

  it('keeps the newer of two answers for one host, whatever order they land in', async () => {
    const list = await liveList('h1')
    const older = parkedRemote()
    const newer = parkedRemote()
    list.status('h1', 'up')
    list.status('h1', 'down')
    list.status('h1', 'up')
    await vi.waitFor(() => expect(readRemote).toHaveBeenCalledTimes(2))

    newer([remoteRow('r-new', 'h1')])
    await vi.waitFor(() => expect(list.held()).toEqual(['a@local', 'r-new@h1']))
    older([remoteRow('r-old', 'h1')])
    await settle()

    expect(list.held()).toEqual(['a@local', 'r-new@h1'])
    list.stop()
  })

  // A host removed meanwhile is gone from the sidebar; a late answer must not bring it back.
  it('brings back no rows from a host removed while its read was out', async () => {
    const list = await liveList('h1')
    const answer = parkedRemote()
    list.status('h1', 'up')
    await vi.waitFor(() => expect(readRemote).toHaveBeenCalledTimes(1))

    list.config.getState().removeHost('h1')
    answer([remoteRow('r-h1', 'h1')])
    await settle()

    expect(list.held()).toEqual(['a@local'])
    list.stop()
  })

  it('brings back no rows from a host removed while its answer waited on a list re-read', async () => {
    const list = await liveList('h1')
    const release = await parkedRequest()
    socket.emit!({ reconnected: true })
    await settle()
    await sent(2)
    readRemote.mockResolvedValueOnce([remoteRow('r-h1', 'h1')])
    list.status('h1', 'up')
    await vi.waitFor(() => expect(readRemote).toHaveBeenCalledTimes(1))
    await settle()

    list.config.getState().removeHost('h1')
    release([p('a', 0)])
    await vi.waitFor(() => expect(list.store.getState().data.status).toBe('success'))

    expect(list.held()).toEqual(['a@local'])
    list.stop()
  })

  it('streams a host only while it is up, and on its return re-reads that host alone', async () => {
    const list = await liveList('h1', 'h2')
    readRemote.mockImplementation(async (hostId) => [remoteRow(`r-${hostId}`, hostId)])
    list.status('h2', 'up')
    list.status('h1', 'up')
    await vi.waitFor(() => expect(list.held()).toEqual(['a@local', 'r-h1@h1', 'r-h2@h2']))
    list.status('h1', 'down')
    expect(socketLog).toEqual(['open local', 'open h2', 'open h1', 'close h1'])
    readRemote.mockClear()

    list.status('h1', 'up')

    expect(socketLog.at(-1)).toBe('open h1')
    await vi.waitFor(() => expect(readRemote.mock.calls).toEqual([['h1']]))
    expect(readLocal).toHaveBeenCalledTimes(1)
    list.stop()
  })

  // Task 8 gap D7: a wire frame names no host, so the socket it came on is its host.
  it('tags a host stream’s frame with that host: never Local, never over a Local row', async () => {
    const list = await liveList('h1')
    readRemote.mockResolvedValue([])
    list.status('h1', 'up')
    await vi.waitFor(() => expect(readRemote).toHaveBeenCalledTimes(1))
    await settle()

    sockets.get('h1')!({ ...wire('a', 0, 'The remote twin'), hostId: 'local' })

    expect(list.held()).toEqual(['a@local', 'a@h1'])
    expect(dataOf(list.store.getState().data)?.[0].name).toBe('A')
    sockets.get('h1')!({ id: 'a', status: 'deleted' })
    expect(list.held()).toEqual(['a@local'])
    list.stop()
  })

  it('a reconnected host stream re-reads that host alone', async () => {
    const list = await liveList('h1')
    readRemote.mockResolvedValue([remoteRow('r-h1', 'h1')])
    list.status('h1', 'up')
    await vi.waitFor(() => expect(readRemote).toHaveBeenCalledTimes(1))

    sockets.get('h1')!({ reconnected: true })

    await vi.waitFor(() => expect(readRemote).toHaveBeenCalledTimes(2))
    await settle()
    expect(readLocal).toHaveBeenCalledTimes(1)
    list.stop()
  })

  it('a frame that lands while its host is read is not rolled back by the older answer', async () => {
    const list = await liveList('h1')
    const answer = parkedRemote()
    list.status('h1', 'up')
    await vi.waitFor(() => expect(readRemote).toHaveBeenCalledTimes(1))

    sockets.get('h1')!(wire('r-new', 1))
    answer([remoteRow('r-h1', 'h1')])

    await vi.waitFor(() => expect(list.held()).toEqual(['a@local', 'r-h1@h1', 'r-new@h1']))
    list.stop()
  })

  // Only a host's own answer since it came up says which projects it has: cached rows, an
  // answer still out or one from before it went away do not. A wait on an unlisted project
  // may end only once every paired host has said.
  it('names a host read once its own answer since it came up has landed', async () => {
    const list = await liveList('h1', 'h2')
    const read = () => [...list.store.getState().readHosts]
    let answer = parkedRemote()
    list.status('h1', 'up')
    await vi.waitFor(() => expect(readRemote).toHaveBeenCalledTimes(1))
    expect(read()).toEqual([])
    answer([remoteRow('r-h1', 'h1')])
    await vi.waitFor(() => expect(read()).toEqual(['h1']))

    answer = parkedRemote()
    sockets.get('h1')!({ reconnected: true })
    expect(read()).toEqual([])
    answer([remoteRow('r-h1', 'h1')])
    await vi.waitFor(() => expect(read()).toEqual(['h1']))

    list.status('h1', 'down')
    expect(read()).toEqual([])
    list.stop()
  })

  it('names no host read whose read failed, or whose answer landed after it went down', async () => {
    const list = await liveList('h1')
    const read = () => [...list.store.getState().readHosts]
    readRemote.mockRejectedValueOnce(new Error('unreachable'))
    list.status('h1', 'up')
    await settle()
    expect(read()).toEqual([])

    list.status('h1', 'down')
    const late = parkedRemote()
    list.status('h1', 'up')
    list.status('h1', 'down')
    late([remoteRow('r-h1', 'h1')])
    await settle()
    expect(read()).toEqual([])
    list.stop()
  })

  // The deleted-project path: each row goes as a tombstone would take it, at once, and the
  // route's "project missing" redirect follows from the list alone.
  it('removing a host takes its rows with it, and nothing late from it comes back', async () => {
    const list = await liveList('h1', 'h2')
    readRemote.mockImplementation(async (hostId) => [
      remoteRow(`r-${hostId}`, hostId),
      remoteRow('shared', hostId),
    ])
    list.status('h1', 'up')
    list.status('h2', 'up')
    await vi.waitFor(() => expect(list.held()).toHaveLength(5))

    list.config.getState().removeHost('h1')

    expect(list.store.getState().data.status).toBe('success')
    expect(list.held()).toEqual(['a@local', 'r-h2@h2', 'shared@h2'])
    sockets.get('h1')!(wire('late', 0))
    expect(list.held()).toEqual(['a@local', 'r-h2@h2', 'shared@h2'])
    list.stop()
  })

  // Held rows are rows whatever the list's load status: a removed host's go even while the
  // local daemon is unreachable or being read, through the same tombstones.
  it.each<[string, (rows: Project[]) => Loadable<Project[]>]>([
    ['failed', (rows) => failed(new Error('the local daemon is down'), success(rows))],
    ['loading', (rows) => loading(success(rows))],
  ])('removing a host takes its held rows while the list is %s', async (_, holding) => {
    const store = await freshStore()
    const { useHostConfigStore } = await import('@/features/hosts/stores/host-config-store')
    useHostConfigStore.setState({ hosts: [host('h1')] })
    const stop = store.getState().startSync()
    store.setState({ data: holding([p('a', 0), remoteRow('only-h1', 'h1')]) })
    const { status } = store.getState().data

    useHostConfigStore.getState().removeHost('h1')

    expect(store.getState().data.status).toBe(status)
    expect(dataOf(store.getState().data)?.map((x) => `${x.id}@${x.hostId}`)).toEqual(['a@local'])
    await settle()
    expect(readLocal).not.toHaveBeenCalled()
    stop()
  })

  it('removing a host from a list never read holds nothing and reads nothing', async () => {
    const store = await freshStore()
    const { useHostConfigStore } = await import('@/features/hosts/stores/host-config-store')
    useHostConfigStore.setState({ hosts: [host('h1')] })
    const stop = store.getState().startSync()

    useHostConfigStore.getState().removeHost('h1')
    await settle()

    expect(store.getState().data.status).toBe('idle')
    expect(readLocal).not.toHaveBeenCalled()
    stop()
  })
})
