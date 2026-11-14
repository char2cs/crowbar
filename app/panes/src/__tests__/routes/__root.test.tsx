import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import {
  createMemoryHistory,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Route as RootRoute } from '@/routes/__root'
import { useDaemonHaltStore } from '@/features/window/stores/daemon-halt-store'
import { OobeScreen } from '@/components/oobe/oobe-screen'
import { ApiError } from '@/lib/errors/api-error'
import * as agentPeer from '@/features/browser/lib/agent-peer'
import { browserApi } from '@/lib/browser-bridge'

const postProject = vi.fn()
vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  fetchPrerequisite: async (tool: string) =>
    tool === 'git' ? { installed: true, version: 'git 2.0' } : { installed: true, authed: true },
  postProject: (...a: unknown[]) => postProject(...a),
}))

const handlers = new Map<string, (event: { payload: unknown }) => void>()

vi.mock('@tauri-apps/api/event', () => ({
  listen: vi.fn(async (name: string, handler: (event: { payload: unknown }) => void) => {
    handlers.set(name, handler)
    return vi.fn()
  }),
}))

const HALT_EVENTS = ['daemon:halted', 'daemon:restarted']
const REASON = 'this home is at layout 9 but this Crowbar only knows layout 0'

type TauriWindow = Window & { __TAURI_INTERNALS__?: object }
const tauriWindow = window as TauriWindow

// The real root, with a stand-in for the OOBE route: the halt must replace
// whatever route is showing, and /oobe is the one with no toast viewport.
function renderRootOn(
  path: string,
  firstRun: () => React.ReactNode = () => <p>first-run screen</p>,
) {
  const oobe = createRoute({ getParentRoute: () => RootRoute, path: '/oobe', component: firstRun })
  const router = createRouter({
    routeTree: RootRoute.addChildren([oobe]),
    history: createMemoryHistory({ initialEntries: [path] }),
  })
  return render(<RouterProvider router={router} />)
}

describe('the root route', () => {
  beforeEach(() => {
    handlers.clear()
    useDaemonHaltStore.getState().clear()
    // A supervisor with no crash loop to report.
    tauriWindow.__TAURI_INTERNALS__ = { invoke: async () => null }
    // The desktop root also subscribes to webview events and lazy-mounts the browser
    // peer, both needing a native shell this fake lacks; when they ran before the file
    // ended depended on load, each throwing an unhandled error.
    for (const on of ['onNavigated', 'onLoading', 'onTitle'] as const) {
      vi.spyOn(browserApi, on).mockResolvedValue(() => {})
    }
    vi.spyOn(agentPeer, 'attachBrowserAgentPeer').mockReturnValue(() => {})
  })
  afterEach(() => {
    delete tauriWindow.__TAURI_INTERNALS__
  })

  it('shows the route while the daemon is serving', async () => {
    renderRootOn('/oobe')

    expect(await screen.findByText('first-run screen')).toBeTruthy()
    await waitFor(() => expect([...handlers.keys()]).toEqual(expect.arrayContaining(HALT_EVENTS)))
    expect(screen.queryByText(/could not start/i)).toBeNull()
  })

  it('replaces the first-run route with the halt screen when the daemon announces a halt', async () => {
    renderRootOn('/oobe')
    await screen.findByText('first-run screen')
    await waitFor(() => expect([...handlers.keys()]).toEqual(expect.arrayContaining(HALT_EVENTS)))

    act(() => handlers.get('daemon:halted')!({ payload: REASON }))

    expect(await screen.findByText(/could not start/i)).toBeTruthy()
    expect(screen.getByText(REASON)).toBeTruthy()
    expect(screen.queryByText('first-run screen')).toBeNull()
  })

  it('returns to the route when the daemon is restarted', async () => {
    renderRootOn('/oobe')
    await waitFor(() => expect([...handlers.keys()]).toEqual(expect.arrayContaining(HALT_EVENTS)))
    act(() => handlers.get('daemon:halted')!({ payload: REASON }))
    await screen.findByText(REASON)

    act(() => handlers.get('daemon:restarted')!({ payload: null }))

    expect(await screen.findByText('first-run screen')).toBeTruthy()
    expect(screen.queryByText(REASON)).toBeNull()
  })

  it('tells the first run why the daemon refused its import', async () => {
    postProject.mockRejectedValue(new ApiError('/srv/app is already a project', 409))
    renderRootOn('/oobe', OobeScreen)

    fireEvent.click(await screen.findByRole('button', { name: /get started/i }))
    const next = await screen.findByRole('button', { name: /continue/i })
    await waitFor(() => expect(next).not.toBeDisabled(), { timeout: 3000 })
    fireEvent.click(next)
    fireEvent.click(await screen.findByRole('button', { name: /choose a folder/i }))
    fireEvent.change(screen.getByLabelText('Space folder'), { target: { value: '/srv/app' } })
    fireEvent.click(screen.getByRole('button', { name: 'Add space' }))

    expect(await screen.findByText('Add “app” failed')).toBeTruthy()
    expect(screen.getByText('/srv/app is already a project')).toBeTruthy()
  })
})
