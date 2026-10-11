import { afterEach, beforeEach, describe, it, expect, vi } from 'vitest'
import { IDBFactory } from 'fake-indexeddb'

import { dataOf, idle, success } from '@/lib/loadable'
import { API_BASE } from '@/lib/api'
import { projectListTransport } from '@/lib/api/project-list-transport'
import { saveCache } from '@/lib/persistence/cache-store'
import { resetDB } from '@/lib/persistence/idb'
import {
  connectProjectList,
  importProjectAndSync,
  seedProjectList,
  useProjectDataStore,
} from '@/lib/store/projects'
import { hostIdForProject } from '@/lib/host-resolve'
import { hostUnavailability } from '@/lib/host-unavailability'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import {
  destroyHostRuntime,
  getHostRuntime,
  getOrCreateHostRuntime,
} from '@/features/hosts/stores/host-runtime-registry'
import { startHostProber } from '@/features/hosts/lib/host-prober'
import { startHostStatus } from '@/features/hosts/lib/host-status'
import type { HostStatus } from '@/features/hosts/types'
import type { Project } from '@/lib/types'

const mockProject: Project = {
  id: 'p1',
  name: 'my-app',
  path: '/repos/my-app',
  lastActivity: new Date(),
  hostId: 'local',
}

const row = (id: string) => ({ id, name: id, path: `/${id}`, lastActivity: '2026-09-28T00:00:00Z' })

const answer = (rows: unknown[]) => ({
  ok: true,
  status: 200,
  json: async () => ({ success: true, data: rows }),
})

// What the desktop proxy answers for a remote host it could not dial.
const unreachable = () => ({
  ok: false,
  status: 502,
  statusText: 'Bad Gateway',
  headers: new Headers({ 'x-crowbar-proxy': 'host-unreachable' }),
  json: async () => null,
})

const serverError = () => ({
  ok: false,
  status: 500,
  statusText: 'Internal Server Error',
  headers: new Headers(),
  json: async () => ({ success: false, error: 'boom' }),
})

type Respond = (init?: RequestInit) => unknown

/** Answers `GET <base>/v0/projects` per base; any other request is a 500. */
function serve(byBase: Record<string, Respond>) {
  const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
    for (const [base, respond] of Object.entries(byBase)) {
      if (url === `${base}/v0/projects`) return respond(init)
    }
    return serverError()
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

/** A host that accepts the request and never answers; only an abort ends it. */
const blackHole =
  (onAbort: () => void): Respond =>
  (init) =>
    new Promise((_resolve, reject) => {
      init?.signal?.addEventListener('abort', () => {
        onAbort()
        reject(new DOMException('aborted', 'AbortError'))
      })
    })

/** An answer that waits until the test releases it. */
function parked(rows: unknown[]): { respond: Respond; release: () => void } {
  let release!: () => void
  const gate = new Promise<void>((resolve) => {
    release = resolve
  })
  return { respond: () => gate.then(() => answer(rows)), release }
}

const paired: string[] = []

function pair(id: string, status?: HostStatus): void {
  useHostConfigStore.getState().addHost({
    id,
    name: id,
    endpoint: `${id}.lan:9443`,
    pairedFingerprint: 'ff',
    clientId: `client-${id}`,
  })
  paired.push(id)
  if (status) getOrCreateHostRuntime(id).getState().setStatus(status)
}

const project = (id: string, hostId: string): Project =>
  ({ ...row(id), hostId }) as unknown as Project

beforeEach(() => {
  // The desktop app: only it reaches a remote host, through a proxy that registers hosts.
  ;(window as unknown as Record<string, unknown>).__TAURI_INTERNALS__ = { invoke: async () => {} }
  // A fresh database per test: the list falls back on its cache when nothing is held.
  globalThis.indexedDB = new IDBFactory()
  resetDB()
  connectProjectList(projectListTransport)
  useHostConfigStore.setState({ hosts: [] })
  useProjectDataStore.setState({ data: idle() })
})

/** The list as a read publishes it: each project with the host it came from. */
async function readList() {
  await useProjectDataStore.getState().fetch()
  return useProjectDataStore.getState().data
}

/** Every held row as `id@host`. */
const held = () =>
  (dataOf(useProjectDataStore.getState().data) ?? []).map((p) => `${p.id}@${p.hostId}`)

afterEach(() => {
  delete (window as unknown as Record<string, unknown>).__TAURI_INTERNALS__
  vi.useRealTimers()
  for (const id of paired.splice(0)) destroyHostRuntime(id)
  useHostConfigStore.setState({ hosts: [] })
  vi.unstubAllGlobals()
})

describe('reading the project list', () => {
  it('reads the local daemon plus every up host, tagging each project with the host it came from', async () => {
    pair('h1', 'up')
    pair('h2', 'down')
    pair('h3')
    const fetchMock = serve({
      [API_BASE]: () => answer([row('p-local')]),
      'crowbar://h1': () => answer([row('p-h1')]),
    })

    await readList()

    await vi.waitFor(() => expect(held()).toEqual(['p-local@local', 'p-h1@h1']))
    const asked = fetchMock.mock.calls.map(([url]) => url)
    expect(asked).not.toContain('crowbar://h2/v0/projects')
    expect(asked).not.toContain('crowbar://h3/v0/projects')
  })

  // A never-paired install asks exactly what it always asked: one GET, nothing else.
  it('reads only the local daemon, with the very same request, where no host was ever paired', async () => {
    const fetchMock = serve({ [API_BASE]: () => answer([row('p-local')]) })

    await readList()

    expect(fetchMock.mock.calls).toEqual([[`${API_BASE}/v0/projects`, undefined]])
    expect(held()).toEqual(['p-local@local'])
  })

  // Boot as the sync engine runs it, over the real transport: the list's live sync, its read,
  // the host status owner. Golden: 71a769f7c (before per-host streams) sends exactly this.
  it('a never-paired install boots with the very request and stream it always had', async () => {
    delete (window as unknown as Record<string, unknown>).__TAURI_INTERNALS__
    const dialled: string[] = []
    vi.stubGlobal(
      'WebSocket',
      class {
        static OPEN = 1
        readyState = 0
        close = () => {}
        constructor(url: string) {
          dialled.push(url)
        }
      },
    )
    const fetchMock = serve({ [API_BASE]: () => answer([row('p-local')]) })

    const stopSync = useProjectDataStore.getState().startSync()
    const read = readList()
    const stopStatus = startHostStatus()
    await read
    await useProjectDataStore.getState().applyDelta(row('p-new'))
    stopStatus()
    stopSync()

    expect(fetchMock.mock.calls).toEqual([[`${API_BASE}/v0/projects`, undefined]])
    expect(dialled.map((url) => new URL(url).pathname)).toEqual(['/v0/projects'])
    expect(held()).toEqual(['p-local@local', 'p-new@local'])
  })

  // An empty success here would read as "no projects": the root route sends
  // the user to /oobe and the cached list is overwritten with nothing.
  it('a local daemon that fails to answer fails the read, never an empty list', async () => {
    pair('h1', 'up')
    serve({
      [API_BASE]: serverError,
      'crowbar://h1': () => answer([row('p-h1')]),
    })

    const list = await readList()
    expect(list.status).toBe('error')
    expect(list.status === 'error' && list.error).toMatchObject({ name: 'ApiError', status: 500 })
  })
})

describe('a paired host’s rows outlive the reads that cannot reach it', () => {
  it('a re-read keeps the rows of a host that went down', async () => {
    pair('h1', 'up')
    serve({
      [API_BASE]: () => answer([row('p-local')]),
      'crowbar://h1': () => answer([row('p-h1')]),
    })
    await readList()
    await vi.waitFor(() => expect(held()).toContain('p-h1@h1'))
    getOrCreateHostRuntime('h1').getState().setStatus('down')

    expect((await readList()).status).toBe('success')

    expect(held()).toEqual(['p-local@local', 'p-h1@h1'])
  })

  it('a re-read keeps the rows of an up host whose read fails', async () => {
    pair('h1', 'up')
    useProjectDataStore.setState({
      data: success([project('p-local', 'local'), project('p-h1', 'h1')]),
    })
    const fetchMock = serve({
      [API_BASE]: () => answer([row('p-local')]),
      'crowbar://h1': unreachable,
    })

    await readList()

    await vi.waitFor(() =>
      expect(fetchMock.mock.calls.map(([url]) => url)).toContain('crowbar://h1/v0/projects'),
    )
    expect(held()).toEqual(['p-local@local', 'p-h1@h1'])
  })

  it('a re-read replaces the rows of the hosts it read, and only theirs', async () => {
    pair('h1', 'up')
    pair('h2', 'down')
    useProjectDataStore.setState({
      data: success([project('p-gone', 'local'), project('p-h1-old', 'h1'), project('p-h2', 'h2')]),
    })
    serve({
      [API_BASE]: () => answer([row('p-local')]),
      'crowbar://h1': () => answer([row('p-h1-new')]),
    })

    await readList()

    await vi.waitFor(() => expect(held()).toContain('p-h1-new@h1'))
    expect(held()).toEqual(['p-local@local', 'p-h1-new@h1', 'p-h2@h2'])
  })

  // Removing a host is not this list's call to make; it just stops carrying what it cannot route.
  it('does not carry the rows of a host that is no longer configured', async () => {
    useProjectDataStore.setState({
      data: success([project('p-local', 'local'), project('p-x', 'h-removed')]),
    })
    serve({ [API_BASE]: () => answer([row('p-local')]) })

    await readList()

    expect(held()).toEqual(['p-local@local'])
  })

  // Boot placement treats the first answer as final: rows that only arrive later are
  // skipped for that boot, so a paired host's cached rows must already be in it.
  it('the first answer already holds the cached rows of a host not probed yet', async () => {
    pair('h1')
    await saveCache('projects-data', 'projects-by-host', [
      project('p-local', 'local'),
      project('p-h1', 'h1'),
    ])
    serve({ [API_BASE]: () => answer([row('p-local')]) })
    const answers: string[][] = []
    const stop = useProjectDataStore.subscribe((s) => {
      if (s.data.status === 'success') answers.push(held())
    })

    await readList()
    stop()

    expect(answers[0]).toEqual(['p-local@local', 'p-h1@h1'])
  })
})

describe('the list order', () => {
  // Each daemon numbers only its own projects, so `order` cannot interleave two hosts.
  it('keeps each host’s rows together, local first, through reads and frames alike', async () => {
    pair('h1', 'up')
    serve({
      [API_BASE]: () =>
        answer([
          { ...row('p-b'), order: 0 },
          { ...row('p-a'), order: 1 },
        ]),
      'crowbar://h1': () => answer([{ ...row('r-x'), order: 0 }]),
    })
    await readList()
    await vi.waitFor(() => expect(held()).toContain('r-x@h1'))

    await useProjectDataStore.getState().applyDelta({ ...row('p-c'), order: 2 })

    expect(held()).toEqual(['p-b@local', 'p-a@local', 'p-c@local', 'r-x@h1'])
  })
})

describe('the local list never waits on a remote host', () => {
  // Boot below the UI, as the sync engine runs it: the list's live sync, its read, the prober.
  // The up host answers its probe only after Local is published, as a slower daemon would.
  it('boots with one paired host up and one down: Local at once, the up host’s rows when it answers', async () => {
    pair('h-up')
    pair('h-down')
    const upHealth = parked([])
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) => {
        if (url === `${API_BASE}/v0/projects`) return answer([row('p-local')])
        if (url === 'crowbar://h-up/v0/health') return upHealth.respond()
        if (url === 'crowbar://h-up/v0/projects') return answer([row('p-up')])
        return blackHole(() => {})(init)
      }),
    )
    connectProjectList({ ...projectListTransport, subscribe: () => () => {} })
    const stopSync = useProjectDataStore.getState().startSync()
    const read = readList()
    const stopProber = startHostProber()
    try {
      expect((await read).status).toBe('success')
      expect(held()).toEqual(['p-local@local'])

      upHealth.release()

      await vi.waitFor(() => expect(held()).toEqual(['p-local@local', 'p-up@h-up']))
      expect(getHostRuntime('h-up')?.getState().status).toBe('up')
      expect(getHostRuntime('h-down')?.getState().status).toBe('probing')
    } finally {
      stopProber()
      stopSync()
    }
  })

  it('publishes the local list while an up host has not answered, and merges that host when it does', async () => {
    pair('h1', 'up')
    const h1 = parked([row('p-h1')])
    serve({ [API_BASE]: () => answer([row('p-local')]), 'crowbar://h1': h1.respond })

    let published = false
    void readList().then(() => (published = true))
    await vi.waitFor(() => expect(published).toBe(true))
    expect(held()).toEqual(['p-local@local'])

    h1.release()
    await vi.waitFor(() => expect(held()).toEqual(['p-local@local', 'p-h1@h1']))
  })

  it('an up host that never answers does not delay the local list, and its read is given up', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    pair('h1', 'up')
    useProjectDataStore.setState({
      data: success([project('p-old', 'local'), project('p-h1', 'h1')]),
    })
    let aborted = false
    serve({
      [API_BASE]: () => answer([row('p-local')]),
      'crowbar://h1': blackHole(() => (aborted = true)),
    })

    let published = false
    void readList().then(() => (published = true))
    await vi.waitFor(() => expect(published).toBe(true))
    expect(held()).toEqual(['p-local@local', 'p-h1@h1'])
    expect(aborted).toBe(false)

    // The bound on a remote project read.
    await vi.advanceTimersByTimeAsync(5_000)
    expect(aborted).toBe(true)
    expect(held()).toEqual(['p-local@local', 'p-h1@h1'])
  })
})

describe('seedProjectList', () => {
  beforeEach(async () => {
    pair('h1')
    await saveCache('projects-data', 'projects-by-host', [
      project('p-gone', 'local'),
      project('p-live', 'local'),
      project('p-h1', 'h1'),
    ])
  })

  // Held so hosts resolve at the first commit, but stale: nothing may conclude from them
  // that a project is gone or kept until the list's own first read has answered.
  it('holds the cached rows as stale until the first read replaces the local ones', async () => {
    await seedProjectList()

    expect(useProjectDataStore.getState().data.status).toBe('loading')
    expect(held()).toEqual(['p-gone@local', 'p-live@local', 'p-h1@h1'])
    expect(hostIdForProject('p-gone')).toBe('local')
    expect(hostUnavailability({ projectId: 'p-gone' })).toBeNull()

    serve({ [API_BASE]: () => answer([row('p-live')]) })
    await readList()

    expect(held()).toEqual(['p-live@local', 'p-h1@h1'])
    expect(hostUnavailability({ projectId: 'p-gone' })).toBe('project-missing')
  })

  it('never replaces a list that is already held', async () => {
    useProjectDataStore.setState({ data: success([project('p-now', 'local')]) })

    await seedProjectList()

    expect(useProjectDataStore.getState().data.status).toBe('success')
    expect(held()).toEqual(['p-now@local'])
  })
})

describe('importProjectAndSync', () => {
  // OOBE navigates to `/` right after an import, and that route reads this
  // list: the import's own answer must already be in it.
  it('puts the imported project in the held list at once', async () => {
    serve({ [API_BASE]: () => answer([]) })
    await useProjectDataStore.getState().fetch()
    importProjectAndSync(mockProject)
    expect(dataOf(useProjectDataStore.getState().data)?.map((p) => p.id)).toEqual(['p1'])
  })

  it('an imported project keeps the host it was created on', async () => {
    pair('h1')
    serve({ [API_BASE]: () => answer([]) })
    await useProjectDataStore.getState().fetch()
    importProjectAndSync({ ...mockProject, id: 'p-remote', hostId: 'h1' })
    const held = dataOf(useProjectDataStore.getState().data)?.find((p) => p.id === 'p-remote')
    expect(held?.hostId).toBe('h1')
  })
})
