import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen } from '@testing-library/react'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
  useParams,
} from '@tanstack/react-router'
import { ShellLanding } from '@/components/layout/shell-landing'
import { useProjectStore } from '@/core/entities/active-project'
import { startStream } from '@/core/stream/stream'
import { reportMigrating, resetConnectionState } from '@/lib/ws/connection-store'
import { projectDTO, resetEntities, seedProjects } from '@/__tests__/__fixtures__/sidebar-entities'

const socket = vi.hoisted(() => ({
  sent: [] as unknown[],
  deliver: (_data: unknown) => {},
  drop: () => {},
}))

vi.mock('@/lib/ws/manager', () => ({
  wsManager: {
    subscribe: (_endpoint: string, cb: (data: unknown) => void) => {
      socket.deliver = cb
      return () => {}
    },
    send: (_endpoint: string, data: unknown) => void socket.sent.push(data),
    onDrop: (_endpoint: string, listener: () => void) => {
      socket.drop = listener
      return () => {}
    },
  },
}))

/** The socket drops and comes back: a projects sub still waiting for its synced failed. */
const reconnect = () =>
  act(() => {
    socket.drop()
    socket.deliver({ t: 'ready', epoch: 'e1' })
  })

// The daemon's boot gate refuses every route but health while it migrates.
const MIGRATING = 'Crowbar is upgrading its data (step 5 of 7: chat-providers-from-history)'

function ProjectHome() {
  const { projectId } = useParams({ strict: false })
  return <p>project home {projectId}</p>
}

// The landing under a bare root, with stand-ins for where it leads.
function renderLanding() {
  const root = createRootRoute()
  const landing = createRoute({ getParentRoute: () => root, path: '/', component: ShellLanding })
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

beforeAll(() => {
  startStream()
})

beforeEach(() => {
  resetEntities()
  resetConnectionState()
  useProjectStore.setState({ activeProjectId: '' })
  reconnect()
  socket.deliver({ t: 'synced', scope: 'projects' })
  resetEntities()
  reconnect()
  socket.sent.length = 0
})

describe('the landing', () => {
  it('waits for the projects scope, then enters the project it names', async () => {
    renderLanding()
    await act(async () => {})
    expect(screen.queryByText(/project home/)).toBeNull()
    expect(screen.queryByText('first-run screen')).toBeNull()

    act(() => seedProjects([projectDTO({ id: 'proj-1' })]))

    expect(await screen.findByText('project home proj-1')).toBeTruthy()
  })

  // Projects imported without a reorder all sit at order 0, so the id decides, as on REST.
  it('with no saved selection, enters the project the sidebar draws first', async () => {
    renderLanding()
    act(() =>
      seedProjects([
        projectDTO({ id: 'b-first-made', order: 0 }),
        projectDTO({ id: 'a-later', order: 0 }),
      ]),
    )
    expect(await screen.findByText('project home a-later')).toBeTruthy()
  })

  it('opens the first-run screen when the daemon has no projects', async () => {
    renderLanding()
    act(() => seedProjects([]))
    expect(await screen.findByText('first-run screen')).toBeTruthy()
  })

  it('shows a migrating daemon as progress and enters the app once the projects land', async () => {
    act(() => reportMigrating({ message: MIGRATING }))
    renderLanding()

    expect(await screen.findByRole('status', { name: MIGRATING })).toBeTruthy()
    expect(screen.queryByRole('button')).toBeNull()

    act(() => seedProjects([projectDTO({ id: 'proj-1' })]))
    expect(await screen.findByText('project home proj-1')).toBeTruthy()
  })

  it('offers Retry once the projects keep failing to load, and Retry asks for them again', async () => {
    renderLanding()
    await act(async () => {})
    reconnect()
    expect(screen.queryByRole('button', { name: /retry/i })).toBeNull()
    reconnect()

    socket.sent.length = 0
    fireEvent.click(await screen.findByRole('button', { name: /retry/i }))
    expect(socket.sent).toEqual([
      { op: 'unsub', scope: 'projects' },
      { op: 'sub', scope: 'projects' },
    ])
    expect(screen.queryByRole('button', { name: /retry/i })).toBeNull()

    act(() => {
      socket.deliver({
        t: 'entity',
        scope: 'projects',
        kind: 'project',
        id: 'proj-1',
        v: 1,
        data: projectDTO({ id: 'proj-1' }),
      })
      socket.deliver({ t: 'synced', scope: 'projects' })
    })
    expect(await screen.findByText('project home proj-1')).toBeTruthy()
  })
})
