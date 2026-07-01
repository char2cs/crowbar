import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { ProjectSwitcherPanel } from '@/components/layout/project-switcher-panel'
import { useProjectStore, useProjectDataStore } from '@/lib/store/projects'
import { success } from '@/lib/loadable'
import { useSidebarNavStore } from '@/features/layout/stores/sidebar-nav'

// Selecting a project navigates to its home route (the route is the source of
// truth); mock the router so navigate is observable without a RouterProvider.
const navigateMock = vi.fn()
vi.mock('@tanstack/react-router', () => ({ useNavigate: () => navigateMock }))

const PROJECTS = [
  { id: 'p1', name: 'Rabbyte', path: '/a', lastActivity: new Date(0) },
  { id: 'p2', name: 'Quiver', path: '/b', lastActivity: new Date(0) },
]

beforeEach(() => {
  navigateMock.mockClear()
  useSidebarNavStore.getState().reset()
  useProjectStore.setState({ activeProjectId: 'p1' })
  // The panel lists the live project set (useProjectDataStore).
  useProjectDataStore.setState({ data: success(PROJECTS) })
})

describe('ProjectSwitcherPanel', () => {
  it('renders a row per project and marks the active one', () => {
    render(<ProjectSwitcherPanel />)
    expect(screen.getByRole('button', { name: /Rabbyte/ })).toHaveAttribute('aria-current', 'true')
    expect(screen.getByRole('button', { name: /Quiver/ })).toHaveAttribute('aria-current', 'false')
  })

  it('switches the active project and pops the stack when a row is clicked', async () => {
    const setActiveProject = vi.spyOn(useProjectStore.getState(), 'setActiveProject')
    useSidebarNavStore.getState().push({ id: 'x', title: 'x', component: null })
    render(<ProjectSwitcherPanel />)
    await userEvent.click(screen.getByRole('button', { name: /Quiver/ }))
    expect(setActiveProject).toHaveBeenCalledWith('p2')
    // The route is navigated to the selected project's home (the desync fix).
    expect(navigateMock).toHaveBeenCalledWith({
      to: '/ide/$projectId/home',
      params: { projectId: 'p2' },
    })
    expect(useSidebarNavStore.getState().stack).toHaveLength(0)
  })

  it('opens the import modal when the import row is clicked', async () => {
    render(<ProjectSwitcherPanel />)
    await userEvent.click(screen.getByRole('button', { name: /Import project/i }))
    // The dialog title is a heading, distinct from the trigger row button.
    // (`getByText` would be ambiguous — the text appears on both.)
    expect(screen.getByRole('heading', { name: 'Import project' })).toBeTruthy()
  })
})
