import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { HostNotResolvedError } from '@/lib/host-path'
import { projectAssetURL, projectBase, projectBaseOnHost, repoBase } from '@/lib/project-url'
import { useProjectDataStore } from '@/lib/store/projects'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { idle, success } from '@/lib/loadable'
import {
  __resetWorkspaceScopesForTest,
  recordWorkspaceScope,
  type WorkspaceScope,
} from '@/lib/workspace-scope'
import * as api from '@/lib/api'
import * as placement from '@/lib/api/sidebar-placement'
import * as agent from '@/features/agent/api/agent-api'
import type { Host } from '@/features/hosts/types'
import type { Project } from '@/lib/types'

const P = 'p1'
const R = 'r1'
const W = 'ws-1'
const C = 'c1'
const F = 'f1'
const SCOPE: WorkspaceScope = { projectId: P, repoId: R, wsId: W, owningChatId: C }

const host = (id: string): Host => ({
  id,
  name: id,
  endpoint: `${id}.example:3737`,
  pairedFingerprint: 'SHA256:aaaa',
  clientId: `client-${id}`,
})
const project = (id: string, hostId: string): Project => ({
  id,
  name: id,
  path: `/work/${id}`,
  lastActivity: new Date(0),
  hostId,
})
const pair = (...ids: string[]) =>
  ids.forEach((id) => useHostConfigStore.getState().addHost(host(id)))
const loadProjects = (...rows: Project[]) => useProjectDataStore.setState({ data: success(rows) })

function resolution(call: () => unknown): HostNotResolvedError {
  try {
    call()
  } catch (err) {
    expect(err).toBeInstanceOf(HostNotResolvedError)
    return err as HostNotResolvedError
  }
  throw new Error('expected HostNotResolvedError, the builder returned')
}

beforeEach(() => {
  // addHost sets a persisted one-way bit; a later test must start never-paired.
  useHostConfigStore.setState({ hosts: [], remoteEverPaired: false })
  useProjectDataStore.setState({ data: idle() })
  __resetWorkspaceScopesForTest()
  recordWorkspaceScope(SCOPE)
})

afterEach(() => {
  delete (window as { __CROWBAR__?: unknown; __TAURI_INTERNALS__?: unknown }).__CROWBAR__
  delete (window as { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__
  vi.unstubAllGlobals()
  vi.resetModules()
})

describe('golden: no remote ever paired (I3) — the bare /v0 strings the literals produced', () => {
  it('projectBase and repoBase', () => {
    expect(projectBase(P)).toBe('/v0/projects/p1')
    expect(repoBase(P, R)).toBe('/v0/projects/p1/repos/r1')
  })

  it('encodes the ids it embeds, as the workspace builders do', () => {
    expect(projectBase('p/1')).toBe('/v0/projects/p%2F1')
    expect(repoBase('p/1', 'r 1')).toBe('/v0/projects/p%2F1/repos/r%201')
  })

  it('does not consult the project list before it loads, even with a local row held', () => {
    loadProjects(project(P, 'local'))
    expect(projectBase(P)).toBe('/v0/projects/p1')
  })

  it('projectAssetURL resolves a DTO icon path against the local daemon, as assetURL did', () => {
    expect(projectAssetURL(P, '/v0/projects/p1/icon?v=2')).toBe(
      api.assetURL('/v0/projects/p1/icon?v=2'),
    )
  })
})

describe('a remote project', () => {
  beforeEach(() => {
    ;(window as { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__ = {}
    pair('host-1', 'host-2')
    loadProjects(project(P, 'host-2'), project('p-local', 'local'))
  })

  it('qualifies both builders with the project’s own host', () => {
    expect(projectBase(P)).toBe('crowbar://host-2/v0/projects/p1')
    expect(repoBase(P, R)).toBe('crowbar://host-2/v0/projects/p1/repos/r1')
  })

  it('stays bare for a local project while a remote is configured', () => {
    expect(projectBase('p-local')).toBe('/v0/projects/p-local')
    expect(repoBase('p-local', 'r9')).toBe('/v0/projects/p-local/repos/r9')
  })

  it('projectAssetURL resolves a DTO icon path against the project’s host, never local', () => {
    expect(projectAssetURL(P, '/v0/projects/p1/icon?v=2')).toBe(
      'crowbar://host-2/v0/projects/p1/icon?v=2',
    )
    expect(projectAssetURL('p-local', '/v0/projects/p-local/icon')).toBe(
      api.assetURL('/v0/projects/p-local/icon'),
    )
  })

  it('projectBaseOnHost names the host itself, for a project created there a moment ago', () => {
    expect(projectBaseOnHost('host-1', 'brand-new')).toBe('crowbar://host-1/v0/projects/brand-new')
    expect(projectBaseOnHost('local', 'brand-new')).toBe('/v0/projects/brand-new')
  })
})

describe('hosts paired but the host cannot be determined: HostNotResolvedError, never local', () => {
  const everyBuilder: [string, () => unknown][] = [
    ['projectBase', () => projectBase(P)],
    ['repoBase', () => repoBase(P, R)],
    ['projectAssetURL', () => projectAssetURL(P, '/v0/projects/p1/icon')],
  ]

  it.each(everyBuilder)('%s while the project list has not loaded', (_n, call) => {
    pair('host-1')
    expect(resolution(call).message).toMatch(/project list is not loaded/)
  })

  it.each(everyBuilder)('%s for a project the list does not hold', (_n, call) => {
    pair('host-1')
    loadProjects(project('other', 'host-1'))
    expect(resolution(call).message).toMatch(/not in the project list/)
  })

  it.each(everyBuilder)('%s when the row names an unconfigured host', (_n, call) => {
    pair('host-1')
    loadProjects(project(P, 'host-gone'))
    expect(resolution(call).message).toMatch(/its host "host-gone" is not configured/)
  })

  it('recovers the moment the project row arrives', () => {
    pair('host-1')
    expect(() => projectBase(P)).toThrow(HostNotResolvedError)
    loadProjects(project(P, 'host-1'))
    expect(projectBase(P)).toBe('crowbar://host-1/v0/projects/p1')
  })

  it('stays unresolved after the last host is revoked (its ids outlive its rows)', () => {
    pair('host-1')
    loadProjects(project(P, 'host-1'))
    useHostConfigStore.getState().removeHost('host-1')
    loadProjects(project('p-local', 'local'))
    expect(resolution(() => repoBase(P, R)).message).toMatch(/not in the project list/)
  })
})

// Every project-id-bearing API call, with the bare path it built before it went through
// the builders. The same table runs under both API bases and with a remote project.
interface Mods {
  api: typeof api
  placement: typeof placement
  agent: typeof agent
}
type Call = (m: Mods) => Promise<unknown>
const CALLS: [name: string, call: Call, method: string, path: string][] = [
  ['fetchRepos', (m) => m.api.fetchRepos(P), 'GET', `/v0/projects/${P}/repos`],
  [
    'fetchWorkspaces',
    (m) => m.api.fetchWorkspaces(P, R),
    'GET',
    `/v0/projects/${P}/repos/${R}/workspaces`,
  ],
  [
    'fetchFolders',
    (m) => m.api.fetchFolders(P, R),
    'GET',
    `/v0/projects/${P}/repos/${R}/chats/folders`,
  ],
  [
    'fetchRepoChats',
    (m) => m.api.fetchRepoChats(P, R),
    'GET',
    `/v0/projects/${P}/repos/${R}/chats`,
  ],
  [
    'fetchWorkspace',
    (m) => m.api.fetchWorkspace(P, R, W),
    'GET',
    `/v0/projects/${P}/repos/${R}/chats/${C}`,
  ],
  ['fetchHomeWorkspace', (m) => m.api.fetchHomeWorkspace(P), 'GET', `/v0/projects/${P}/home`],
  ['fetchHomeChats', (m) => m.api.fetchHomeChats(P), 'GET', `/v0/projects/${P}/home/chats`],
  [
    'fetchHomeFolders',
    (m) => m.api.fetchHomeFolders(P),
    'GET',
    `/v0/projects/${P}/home/chats/folders`,
  ],
  ['postRepo', (m) => m.api.postRepo(P, 'n', '/x'), 'POST', `/v0/projects/${P}/repos`],
  ['renameRepo', (m) => m.api.renameRepo(P, R, 'n'), 'PATCH', `/v0/projects/${P}/repos/${R}`],
  ['renameProject', (m) => m.api.renameProject(P, 'n'), 'PATCH', `/v0/projects/${P}`],
  ['deleteProject', (m) => m.api.deleteProject(P), 'DELETE', `/v0/projects/${P}`],
  [
    'getRepoPullRequests',
    (m) => m.api.getRepoPullRequests(P, R),
    'GET',
    `/v0/projects/${P}/repos/${R}/pull-requests`,
  ],
  [
    'importBranches',
    (m) => m.api.importBranches(P, R, ['b']),
    'POST',
    `/v0/projects/${P}/repos/${R}/chats/import-batch`,
  ],
  ['deleteRepo', (m) => m.api.deleteRepo(P, R), 'DELETE', `/v0/projects/${P}/repos/${R}`],
  [
    'createFolder',
    (m) => m.placement.createFolder(P, R, 'n', ''),
    'POST',
    `/v0/projects/${P}/repos/${R}/chats/folders`,
  ],
  [
    'createHomeFolder',
    (m) => m.placement.createHomeFolder(P, 'n', ''),
    'POST',
    `/v0/projects/${P}/home/chats/folders`,
  ],
  [
    'placeFolder',
    (m) => m.placement.placeFolder(P, R, F, { order: 1 }),
    'PATCH',
    `/v0/projects/${P}/repos/${R}/chats/folders/${F}`,
  ],
  [
    'placeHomeFolder',
    (m) => m.placement.placeHomeFolder(P, F, { order: 1 }),
    'PATCH',
    `/v0/projects/${P}/home/chats/folders/${F}`,
  ],
  [
    'deleteFolder',
    (m) => m.placement.deleteFolder(P, R, F),
    'DELETE',
    `/v0/projects/${P}/repos/${R}/chats/folders/${F}`,
  ],
  [
    'deleteHomeFolder',
    (m) => m.placement.deleteHomeFolder(P, F),
    'DELETE',
    `/v0/projects/${P}/home/chats/folders/${F}`,
  ],
  [
    'placeRepo',
    (m) => m.placement.placeRepo(P, R, { order: 1 }),
    'PATCH',
    `/v0/projects/${P}/repos/${R}`,
  ],
  ['placeProject', (m) => m.placement.placeProject(P, 1), 'PATCH', `/v0/projects/${P}`],
  [
    'createChatWithOwnWorktree',
    (m) => m.agent.createChatWithOwnWorktree(P, R, 'claude'),
    'POST',
    `/v0/projects/${P}/repos/${R}/chats`,
  ],
]

const fetchMock = vi.fn()
function stubFetch(): void {
  fetchMock.mockReset()
  fetchMock.mockImplementation(
    async () => new Response(JSON.stringify({ success: true, data: null }), { status: 200 }),
  )
  vi.stubGlobal('fetch', fetchMock)
}
/** What a call asked the network for. Shape errors after the request (the stub answers null) are not the point. */
async function requested(call: () => Promise<unknown>): Promise<[string, string]> {
  await call().catch(() => undefined)
  const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit | undefined]
  return [url, init?.method ?? 'GET']
}

describe.each([
  ['browser', ''],
  ['desktop', 'crowbar://localhost'],
])('golden: every project-scoped call under the %s API base', (_name, base) => {
  beforeEach(stubFetch)

  it.each(CALLS)('%s', async (_n, call, method, path) => {
    vi.resetModules()
    ;(window as { __CROWBAR__?: unknown }).__CROWBAR__ = { api: base }
    const mods: Mods = {
      api: await import('@/lib/api'),
      placement: await import('@/lib/api/sidebar-placement'),
      agent: await import('@/features/agent/api/agent-api'),
    }
    ;(await import('@/lib/workspace-scope')).recordWorkspaceScope(SCOPE)
    expect(mods.api.API_BASE).toBe(base)
    expect(await requested(() => call(mods))).toEqual([`${base}${path}`, method])
  })
})

describe('a remote project: every project-scoped call lands on its own host', () => {
  const mods: Mods = { api, placement, agent }
  beforeEach(() => {
    stubFetch()
    ;(window as { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__ = {}
    pair('host-2')
    loadProjects(project(P, 'host-2'))
  })

  it.each(CALLS)('%s', async (_n, call, method, path) => {
    expect(await requested(() => call(mods))).toEqual([`crowbar://host-2${path}`, method])
  })

  it('a local project’s call stays on the local daemon while a remote is configured', async () => {
    loadProjects(project(P, 'local'), project('p2', 'host-2'))
    expect(await requested(() => api.fetchRepos(P))).toEqual(['/v0/projects/p1/repos', 'GET'])
  })
})

describe('hosts paired but the project list not loaded: a rejection, no request, never local', () => {
  const mods: Mods = { api, placement, agent }
  beforeEach(() => {
    stubFetch()
    pair('host-2')
  })

  it.each(CALLS)('%s rejects (not a synchronous throw) and sends nothing', async (_n, call) => {
    // Callers chain .then/.catch on these; a throw before the promise exists would skip all of it.
    const pending = call(mods)
    await expect(pending).rejects.toBeInstanceOf(HostNotResolvedError)
    expect(fetchMock).not.toHaveBeenCalled()
  })
})

// project-url sits in an import cycle with the transport (its icon URLs go through api.ts,
// whose project calls go through it), so each member must work when it is the first loaded,
// and desktop's API_BASE is itself `crowbar://localhost`, which must never reach qualify.
const CYCLE = ['@/lib/api', '@/lib/project-url', '@/lib/crowbar-bridge', '@/lib/ws/url']
describe.each([
  ['browser', ''],
  ['desktop', 'crowbar://localhost'],
])('under the %s API base', (_name, apiBase) => {
  it.each(CYCLE)('the builders are unchanged when %s loads first', async (entry) => {
    vi.resetModules()
    ;(window as { __CROWBAR__?: unknown }).__CROWBAR__ = { api: apiBase }
    await import(/* @vite-ignore */ entry)
    const urls = await import('@/lib/project-url')
    expect((await import('@/lib/api')).API_BASE).toBe(apiBase)
    expect(urls.projectBase(P)).toBe('/v0/projects/p1')
    expect(urls.repoBase(P, R)).toBe('/v0/projects/p1/repos/r1')
    expect(urls.projectAssetURL(P, '/v0/projects/p1/icon')).toBe(`${apiBase}/v0/projects/p1/icon`)
  })
})
