import { beforeEach, describe, expect, it } from 'vitest'
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
import { projectDTO, resetEntities, seedProjects } from '@/__tests__/__fixtures__/sidebar-entities'

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

beforeEach(() => {
  resetEntities()
  useProjectStore.setState({ activeProjectId: '' })
})

describe('the landing', () => {
  it('waits for the projects scope, then enters the project it names', async () => {
    renderLanding()
    expect(screen.queryByText(/project home/)).toBeNull()

    act(() => seedProjects([projectDTO({ id: 'proj-1' })]))

    expect(await screen.findByText('project home proj-1')).toBeTruthy()
  })

  it('opens the first-run screen when the daemon has no projects', async () => {
    renderLanding()
    act(() => seedProjects([]))
    expect(await screen.findByText('first-run screen')).toBeTruthy()
  })
})
