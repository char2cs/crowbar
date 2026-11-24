import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, render, screen } from '@testing-library/react'
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
import { pairRemoteHost, resetHostWorld } from '@/__tests__/__fixtures__/remote-host'
import { projectDTO, seedProjects } from '@/__tests__/__fixtures__/sidebar-entities'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { resetWindowPaneStoreForTests } from '@/__tests__/__fixtures__/window-pane-store'
import { seedChatPaneRecord } from '@/__tests__/__fixtures__/view-state'

// Which chats the last run's placements vouch for is host-placements' business, not the landing's.
vi.mock('@/lib/host-resolve-cached', () => ({
  isChatRecalled: (chatId: string) => chatId === 'c-recalled',
}))

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
  const addHost = createRoute({
    getParentRoute: () => root,
    path: '/add-host',
    component: () => <p>add a host</p>,
  })
  const router = createRouter({
    routeTree: root.addChildren([landing, home, addHost]),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  return render(<RouterProvider router={router} />)
}

beforeEach(() => {
  resetHostWorld()
  useProjectStore.setState({ activeProjectId: '' })
})

afterEach(() => resetHostWorld())

describe('the landing', () => {
  it('with no host paired, sends the install to the add-host screen', async () => {
    renderLanding()
    expect(await screen.findByText('add a host')).toBeTruthy()
  })

  it('waits on the paired hosts, then enters the project the first one lists', async () => {
    pairRemoteHost('host-1', 'Studio')
    renderLanding()
    expect(await screen.findByText(/waiting for your hosts/i)).toBeTruthy()
    expect(screen.queryByText(/project home/)).toBeNull()

    act(() => seedProjects([projectDTO({ id: 'proj-1' })]))

    expect(await screen.findByText('project home proj-1')).toBeTruthy()
  })

  // Projects imported without a reorder all sit at order 0, so the id decides, as on REST.
  it('with no saved selection, enters the project the list draws first', async () => {
    pairRemoteHost()
    seedProjects([
      projectDTO({ id: 'b-first-made', order: 0 }),
      projectDTO({ id: 'a-later', order: 0 }),
    ])
    renderLanding()
    expect(await screen.findByText('project home a-later')).toBeTruthy()
  })

  it('enters the saved project when its host lists it', async () => {
    pairRemoteHost()
    seedProjects([projectDTO({ id: 'p1', order: 0 }), projectDTO({ id: 'p2', order: 1 })])
    useProjectStore.setState({ activeProjectId: 'p2' })
    renderLanding()
    expect(await screen.findByText('project home p2')).toBeTruthy()
  })
})

describe('the landing of a phone with a paired host that has not listed', () => {
  const WAITING = /Waiting for your hosts to list their projects/

  beforeEach(() => {
    pairRemoteHost('h1', 'rig')
    resetWindowPaneStoreForTests()
  })
  afterEach(() => resetWindowPaneStoreForTests())

  it('says it waits for the hosts when no chat is open', async () => {
    renderLanding()
    expect(await screen.findByText(WAITING)).toBeTruthy()
  })

  // The chat is already on screen from the last run's placement; the panel is for the empty stage.
  it('does not stack the waiting panel under a chat recalled from the last run', async () => {
    seedChatPaneRecord(windowPaneStore, 'pane-1', 'c-recalled', null)
    renderLanding()
    await act(async () => {})
    expect(screen.queryByText(WAITING)).toBeNull()
  })
})
