import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { ProjectSwitcherPanel } from '@/components/layout/project-switcher-panel'
import { useProjectStore } from '@/lib/store/projects'
import { useSidebarNavStore } from '@/features/layout/stores/sidebar-nav'

const PROJECTS = [
  { id: 'p1', name: 'Rabbyte', path: '/a', lastActivity: new Date(0) },
  { id: 'p2', name: 'Quiver', path: '/b', lastActivity: new Date(0) },
]

beforeEach(() => {
  useSidebarNavStore.getState().reset()
  useProjectStore.setState({ projects: PROJECTS, activeProjectId: 'p1' })
})

describe('ProjectSwitcherPanel', () => {
  it('renders a row per project and marks the active one', () => {
    render(<ProjectSwitcherPanel />)
    expect(screen.getByRole('button', { name: /Rabbyte/ })).toHaveAttribute(
      'aria-current',
      'true',
    )
    expect(screen.getByRole('button', { name: /Quiver/ })).toHaveAttribute(
      'aria-current',
      'false',
    )
  })

  it('switches the active project and pops the stack when a row is clicked', async () => {
    const setActiveProject = vi.spyOn(useProjectStore.getState(), 'setActiveProject')
    useSidebarNavStore.getState().push({ id: 'x', title: 'x', component: null })
    render(<ProjectSwitcherPanel />)
    await userEvent.click(screen.getByRole('button', { name: /Quiver/ }))
    expect(setActiveProject).toHaveBeenCalledWith('p2')
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
