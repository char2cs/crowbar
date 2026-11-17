import { act, render, screen } from '@testing-library/react'
import {
  createMemoryHistory,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { beforeEach, describe, expect, it } from 'vitest'
import { Route as RootRoute } from '@/routes/__root'
import { useDaemonHaltStore } from '@/features/window/stores/daemon-halt-store'

const REASON = 'this home is at layout 9 but this Crowbar only knows layout 0'

// The real root, with a stand-in route: the halt must replace whatever route is showing.
function renderRootOn(path: string) {
  const landing = createRoute({
    getParentRoute: () => RootRoute,
    path: '/add-host',
    component: () => <p>landing screen</p>,
  })
  const router = createRouter({
    routeTree: RootRoute.addChildren([landing]),
    history: createMemoryHistory({ initialEntries: [path] }),
  })
  return render(<RouterProvider router={router} />)
}

describe('the root route', () => {
  beforeEach(() => {
    useDaemonHaltStore.getState().clear()
  })

  it('shows the route while the daemon is serving', async () => {
    renderRootOn('/add-host')

    expect(await screen.findByText('landing screen')).toBeTruthy()
    expect(screen.queryByText(/could not start/i)).toBeNull()
  })

  it('replaces the route with the halt screen when a daemon refuses to open its home', async () => {
    renderRootOn('/add-host')
    await screen.findByText('landing screen')

    act(() => useDaemonHaltStore.getState().halt(REASON))

    expect(await screen.findByText(/could not start/i)).toBeTruthy()
    expect(screen.getByText(REASON)).toBeTruthy()
    expect(screen.queryByText('landing screen')).toBeNull()
  })

  it('returns to the route once the halt is cleared', async () => {
    renderRootOn('/add-host')
    await screen.findByText('landing screen')
    act(() => useDaemonHaltStore.getState().halt(REASON))
    await screen.findByText(REASON)

    act(() => useDaemonHaltStore.getState().clear())

    expect(await screen.findByText('landing screen')).toBeTruthy()
    expect(screen.queryByText(REASON)).toBeNull()
  })
})
