import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  isRedirect,
  RouterProvider,
  useParams,
} from '@tanstack/react-router'
import { Route } from '@/routes/_shell/index'
import { ShellLanding } from '@/components/layout/shell-landing'
import { useProjectDataStore, useProjectStore } from '@/lib/store/projects'
import { idle } from '@/lib/loadable'
import { getDB } from '@/lib/persistence/idb'
import { resetConnectionState } from '@/lib/ws/connection-store'

// Blackbox integration test for the cold-start landing race (the real bug found
// in the E2E run): at app launch the daemon sidecar's socket may not be
// accepting yet, so the landing route's first GETs reject at the transport
// layer. Before the fix this either crashed the route (uncaught fetchProjects)
// or fell through to the "No repositories yet" empty state. This test drives the
// ACTUAL Route.beforeLoad through a transport that rejects a few times before
// answering, and asserts the user is redirected onto Project Home.

type FetchResult = {
  ok: boolean
  status: number
  statusText?: string
  json: () => Promise<unknown>
}
const envelope = (data: unknown): FetchResult => ({
  ok: true,
  status: 200,
  json: async () => ({ success: true, data }),
})

const PROJECT = { id: 'proj-1', name: 'crowbar', path: '/repos/crowbar' }

// The daemon's boot gate refuses every route but health while it migrates.
const MIGRATING = 'Crowbar is upgrading its data (step 5 of 7: chat-providers-from-history)'
const refusal = (status: number, code: string, error: string): FetchResult => ({
  ok: false,
  status,
  json: async () => ({ success: false, error, code }),
})

function routeByPath(path: string): FetchResult {
  if (path.endsWith('/v0/projects')) return envelope([PROJECT])
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

beforeEach(async () => {
  useProjectStore.setState({ activeProjectId: '' })
  useProjectDataStore.setState({ data: idle() })
  resetConnectionState()
  await (await getDB()).clear('projects-data')
})

function ProjectHome() {
  const { projectId } = useParams({ strict: false })
  return <p>project home {projectId}</p>
}

// The real landing route under a bare root, with stand-ins for where it leads.
function renderLanding() {
  const root = createRootRoute()
  const landing = createRoute({
    getParentRoute: () => root,
    path: '/',
    beforeLoad: Route.options.beforeLoad as () => Promise<void>,
    component: ShellLanding,
  })
  const home = createRoute({
    getParentRoute: () => root,
    path: '/ide/$projectId/home',
    component: ProjectHome,
  })
  const oobe = createRoute({
    getParentRoute: () => root,
    path: '/oobe',
    component: () => <p>first-run screen</p>,
  })
  const router = createRouter({
    routeTree: root.addChildren([landing, home, oobe]),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  return render(<RouterProvider router={router} />)
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('landing route cold-start resilience', () => {
  it('retries through transient transport failures and redirects to Project Home', async () => {
    // The first two attempts on the projects endpoint reject (socket not accepting),
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
    expect(opts.to).toBe('/ide/$projectId/home')
    expect(opts.params).toMatchObject({
      projectId: 'proj-1',
    })
  })

  it('redirects to /oobe when the daemon answers with zero projects', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => envelope([])),
    )

    const result = await runBeforeLoad()

    expect(isRedirect(result)).toBe(true)
    expect((result as { options: { to: string } }).options.to).toBe('/oobe')
  })

  it('shows a migrating daemon as progress and enters the app once the stream is back', async () => {
    let serving = false
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        serving ? envelope([PROJECT]) : refusal(503, 'daemon_migrating', MIGRATING),
      ),
    )

    renderLanding()

    expect(await screen.findByRole('status', { name: MIGRATING })).toBeTruthy()
    expect(screen.queryByRole('button')).toBeNull()

    serving = true
    // What the projects stream delivers when its refused socket finally opens.
    await useProjectDataStore.getState().applyDelta({ reconnected: true })

    expect(await screen.findByText('project home proj-1')).toBeTruthy()
  })

  it('re-reads the projects on Retry after a failed first load', async () => {
    let answered = false
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        if (answered) return envelope([PROJECT])
        answered = true
        return refusal(500, 'internal', 'the projects ledger is unreadable')
      }),
    )

    renderLanding()

    fireEvent.click(await screen.findByRole('button', { name: /retry/i }))

    expect(await screen.findByText('project home proj-1')).toBeTruthy()
  })
})
