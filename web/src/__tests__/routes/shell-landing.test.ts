import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { isRedirect } from '@tanstack/react-router'
import { Route } from '@/routes/_shell/index'
import { useProjectStore } from '@/lib/store/projects'

// Blackbox integration test for the cold-start landing race (the real bug found
// in the E2E run): at app launch the daemon sidecar's socket may not be
// accepting yet, so the landing route's first GETs reject at the transport
// layer. Before the fix this either crashed the route (uncaught fetchProjects)
// or fell through to the "No repositories yet" empty state. This test drives the
// ACTUAL Route.beforeLoad through a transport that rejects a few times before
// answering, and asserts the user is redirected onto their workspace.

type FetchResult = { ok: boolean; status: number; statusText?: string; json: () => Promise<unknown> }
const envelope = (data: unknown): FetchResult => ({
  ok: true,
  status: 200,
  json: async () => ({ success: true, data }),
})

const PROJECT = { id: 'proj-1', name: 'crowbar', path: '/repos/crowbar' }
const REPO = { id: 'repo-1', projectId: 'proj-1', name: 'crowbar' }
const WORKSPACE = { id: 'ws-1', repoId: 'repo-1', branch: 'develop', status: 'ready' }

function routeByPath(path: string): FetchResult {
  if (path.endsWith('/v0/projects')) return envelope([PROJECT])
  if (path.endsWith(`/v0/projects/${PROJECT.id}/repos`)) return envelope([REPO])
  if (path.includes(`/repos/${REPO.id}/workspaces`)) return envelope([WORKSPACE])
  throw new Error(`unexpected path ${path}`)
}

async function runBeforeLoad(): Promise<unknown> {
  const beforeLoad = Route.options.beforeLoad as () => Promise<void>
  try {
    await beforeLoad()
    return { redirected: false }
  } catch (err) {
    return err
  }
}

beforeEach(() => {
  useProjectStore.setState({ activeProjectId: '' })
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('landing route cold-start resilience', () => {
  it('retries through transient transport failures and redirects to the workspace', async () => {
    // The first two attempts on EVERY endpoint reject (socket not accepting),
    // then succeed — mirroring the sidecar's bind window.
    const failsLeft = new Map<string, number>()
    const fetchMock = vi.fn(async (url: string) => {
      const key = url.replace(/^crowbar:\/\/localhost/, '')
      const remaining = failsLeft.get(key) ?? 2
      if (remaining > 0) {
        failsLeft.set(key, remaining - 1)
        throw new TypeError('Load failed')
      }
      return routeByPath(url)
    })
    vi.stubGlobal('fetch', fetchMock)

    const result = await runBeforeLoad()

    expect(isRedirect(result)).toBe(true)
    const opts = (result as { options: { to: string; params: Record<string, string> } }).options
    expect(opts.to).toBe('/ide/$projectId/$repoId/$wsId')
    expect(opts.params).toMatchObject({
      projectId: 'proj-1',
      repoId: 'repo-1',
      wsId: 'ws-1',
    })
  })

  it('redirects to /oobe when the daemon answers with zero projects', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => envelope([])))

    const result = await runBeforeLoad()

    expect(isRedirect(result)).toBe(true)
    expect((result as { options: { to: string } }).options.to).toBe('/oobe')
  })
})
