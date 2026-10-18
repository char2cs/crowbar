import { act, render, screen, waitFor } from '@testing-library/react'
import {
  createMemoryHistory,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Route as RootRoute } from '@/routes/__root'
import { useDaemonHaltStore } from '@/features/window/stores/daemon-halt-store'
import { apiFetch } from '@/lib/api'

// The sync engine needs a daemon; the root's halt handling does not.
vi.mock('@/components/app-sync-provider', () => ({
  AppSyncProvider: ({ children }: { children: React.ReactNode }) => children,
}))

vi.mock('@/lib/api', () => ({ apiFetch: vi.fn() }))

const handlers = new Map<string, (event: { payload: unknown }) => void>()

vi.mock('@tauri-apps/api/event', () => ({
  listen: vi.fn(async (name: string, handler: (event: { payload: unknown }) => void) => {
    handlers.set(name, handler)
    return vi.fn()
  }),
}))

const REASON = 'this home is at layout 9 but this Crowbar only knows layout 0'

type TauriWindow = Window & { __TAURI_INTERNALS__?: object }
const tauriWindow = window as TauriWindow

// The real root, with a stand-in for the OOBE route: the halt must replace
// whatever route is showing, and /oobe is the one with no toast viewport.
function renderRootOn(path: string) {
  const oobe = createRoute({
    getParentRoute: () => RootRoute,
    path: '/oobe',
    component: () => <p>first-run screen</p>,
  })
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
    vi.mocked(apiFetch).mockResolvedValue({ status: 'ok' })
    tauriWindow.__TAURI_INTERNALS__ = {}
  })
  afterEach(() => {
    delete tauriWindow.__TAURI_INTERNALS__
  })

  it('shows the route while the daemon is serving', async () => {
    renderRootOn('/oobe')

    expect(await screen.findByText('first-run screen')).toBeTruthy()
    await waitFor(() => expect(handlers.size).toBe(4))
    expect(screen.queryByText(/could not start/i)).toBeNull()
  })

  it('replaces the first-run route with the halt screen when the daemon announces a halt', async () => {
    renderRootOn('/oobe')
    await screen.findByText('first-run screen')
    await waitFor(() => expect(handlers.size).toBe(4))

    act(() => handlers.get('daemon:halted')!({ payload: REASON }))

    expect(await screen.findByText(/could not start/i)).toBeTruthy()
    expect(screen.getByText(REASON)).toBeTruthy()
    expect(screen.queryByText('first-run screen')).toBeNull()
  })

  it('shows the halt screen when the daemon was already halted before the page mounted', async () => {
    vi.mocked(apiFetch).mockResolvedValue({ status: 'halted', detail: { message: REASON } })

    renderRootOn('/oobe')

    expect(await screen.findByText(REASON)).toBeTruthy()
    expect(screen.queryByText('first-run screen')).toBeNull()
  })

  it('returns to the route when the daemon is restarted', async () => {
    renderRootOn('/oobe')
    await waitFor(() => expect(handlers.size).toBe(4))
    act(() => handlers.get('daemon:halted')!({ payload: REASON }))
    await screen.findByText(REASON)

    act(() => handlers.get('daemon:restarted')!({ payload: null }))

    expect(await screen.findByText('first-run screen')).toBeTruthy()
    expect(screen.queryByText(REASON)).toBeNull()
  })
})
