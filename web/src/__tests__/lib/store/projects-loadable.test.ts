// The `/v0/projects` socket answers every subscribe with a snapshot — one
// complete ProjectDTO frame per project — and then pushes one complete DTO per
// change. Those frames ARE the list: merging them costs no request, so a boot
// makes exactly one GET /v0/projects however the snapshot and the seed GET
// interleave.
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { resetDB } from '@/lib/persistence/idb'
import { dataOf } from '@/lib/loadable'
import type { Project } from '@/lib/types'

const { fetchAllProjects, socket } = vi.hoisted(() => ({
  fetchAllProjects: vi.fn<() => Promise<Project[]>>(),
  socket: { emit: null as null | ((frame: unknown) => void) },
}))

vi.mock('@/lib/api', () => ({ fetchAllProjects }))
vi.mock('@/lib/ws/manager', () => ({
  wsManager: {
    subscribe: (_endpoint: string, cb: (frame: unknown) => void) => {
      socket.emit = cb
      return () => {
        socket.emit = null
      }
    },
    send: () => {},
  },
}))

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
  fetchAllProjects.mockImplementationOnce(() => answer)
  return release
}

async function sent(times: number): Promise<void> {
  await vi.waitFor(() => expect(fetchAllProjects).toHaveBeenCalledTimes(times))
}

async function settle(): Promise<void> {
  // Past DELTA_DEBOUNCE_MS: any re-read a frame scheduled would have been sent.
  await vi.advanceTimersByTimeAsync(500)
}

async function freshStore() {
  vi.resetModules()
  const { useProjectDataStore } = await import('@/lib/store/projects')
  return useProjectDataStore
}

beforeEach(() => {
  resetDB()
  fetchAllProjects.mockReset()
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
})
afterEach(() => {
  vi.useRealTimers()
})

describe('useProjectDataStore', () => {
  it('fetch populates loadable success', async () => {
    const store = await freshStore()
    fetchAllProjects.mockResolvedValueOnce([p('p1', 0)])
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
    expect(fetchAllProjects).toHaveBeenCalledTimes(1)
    expect(dataOf(store.getState().data)?.map((x) => x.id)).toEqual(['a', 'b'])
  })

  it('a snapshot landing after the seed GET costs no second request and no re-render', async () => {
    const store = await freshStore()
    fetchAllProjects.mockResolvedValueOnce([p('a', 0), p('b', 1)])
    await store.getState().fetch()
    const seeded = store.getState().data
    store.getState().startSync()
    socket.emit!(wire('a', 0))
    socket.emit!(wire('b', 1))
    await settle()
    expect(fetchAllProjects).toHaveBeenCalledTimes(1)
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
    expect(fetchAllProjects).toHaveBeenCalledTimes(1)
  })

  it('live frames upsert, reorder and tombstone without a request', async () => {
    const store = await freshStore()
    fetchAllProjects.mockResolvedValueOnce([p('a', 0), p('b', 1)])
    await store.getState().fetch()
    store.getState().startSync()
    socket.emit!(wire('c', 2))
    socket.emit!(wire('b', 0))
    socket.emit!(wire('a', 1))
    socket.emit!({ id: 'c', status: 'deleted' })
    await settle()
    expect(fetchAllProjects).toHaveBeenCalledTimes(1)
    expect(dataOf(store.getState().data)?.map((x) => x.id)).toEqual(['b', 'a'])
    expect(store.getState().data.status).toBe('success')
  })

  // The only socket feeding this list is the local daemon's; a frame merged
  // untagged would leave a project with no host for any caller to route by.
  it('a frame off the project socket is tagged with the local host it came from', async () => {
    const store = await freshStore()
    fetchAllProjects.mockResolvedValueOnce([p('a', 0)])
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
    fetchAllProjects.mockResolvedValue([p('a', 0)])
    await store.getState().fetch()
    store.getState().startSync()
    socket.emit!({ reconnected: true })
    await settle()
    await vi.waitFor(() => expect(fetchAllProjects).toHaveBeenCalledTimes(2))
  })
})
