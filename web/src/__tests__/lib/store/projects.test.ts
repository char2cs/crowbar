import { afterEach, beforeEach, describe, it, expect, vi } from 'vitest'

// api.ts imports isTauri from crowbar-bridge, so this mock's factory runs
// before this file's own top-level statements — vi.hoisted makes it exist by
// then. Tauri is on: only the desktop app can reach a remote host at all.
const { isTauri } = vi.hoisted(() => ({ isTauri: vi.fn(() => true) }))
vi.mock('@/lib/crowbar-bridge', () => ({ isTauri }))

import { dataOf } from '@/lib/loadable'
import { API_BASE, fetchAllProjects } from '@/lib/api'
import { importProjectAndSync, useProjectDataStore } from '@/lib/store/projects'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import {
  destroyHostRuntime,
  getOrCreateHostRuntime,
} from '@/features/hosts/stores/host-runtime-registry'
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

/** Answers `GET <base>/v0/projects` per base; any other request is a 500. */
function serve(byBase: Record<string, () => unknown>) {
  const fetchMock = vi.fn(async (url: string) => {
    for (const [base, respond] of Object.entries(byBase)) {
      if (url === `${base}/v0/projects`) return respond()
    }
    return serverError()
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
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

beforeEach(() => {
  useHostConfigStore.setState({ hosts: [] })
})

afterEach(() => {
  for (const id of paired.splice(0)) destroyHostRuntime(id)
  useHostConfigStore.setState({ hosts: [] })
  vi.unstubAllGlobals()
})

describe('fetchAllProjects', () => {
  it('reads the local daemon plus every up host, tagging each project with the host it came from', async () => {
    pair('h1', 'up')
    pair('h2', 'down')
    pair('h3')
    const fetchMock = serve({
      [API_BASE]: () => answer([row('p-local')]),
      'crowbar://h1': () => answer([row('p-h1')]),
    })

    const projects = await fetchAllProjects()

    expect(projects.map((p) => [p.id, p.hostId])).toEqual([
      ['p-local', 'local'],
      ['p-h1', 'h1'],
    ])
    const asked = fetchMock.mock.calls.map(([url]) => url)
    expect(asked).not.toContain('crowbar://h2/v0/projects')
    expect(asked).not.toContain('crowbar://h3/v0/projects')
  })

  it('an up host that fails to answer drops only its own projects', async () => {
    pair('h1', 'up')
    serve({
      [API_BASE]: () => answer([row('p-local')]),
      'crowbar://h1': unreachable,
    })

    const projects = await fetchAllProjects()

    expect(projects.map((p) => p.id)).toEqual(['p-local'])
  })

  // An empty success here would read as "no projects": the root route sends
  // the user to /oobe and the cached list is overwritten with nothing.
  it('a local daemon that fails to answer fails the read, never an empty list', async () => {
    pair('h1', 'up')
    serve({
      [API_BASE]: serverError,
      'crowbar://h1': () => answer([row('p-h1')]),
    })

    await expect(fetchAllProjects()).rejects.toMatchObject({ name: 'ApiError', status: 500 })
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
    serve({ [API_BASE]: () => answer([]) })
    await useProjectDataStore.getState().fetch()
    importProjectAndSync({ ...mockProject, id: 'p-remote', hostId: 'h1' })
    const held = dataOf(useProjectDataStore.getState().data)?.find((p) => p.id === 'p-remote')
    expect(held?.hostId).toBe('h1')
  })
})
