import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi } from 'vitest'
import type { Project } from '@/lib/types'
import { SidebarProjectSwitcher } from '@/components/layout/sidebar-project-switcher'

const project = (id: string, name = id): Project => ({
  id,
  name,
  path: `/repos/${id}`,
  lastActivity: new Date('2026-08-28T00:00:00Z'),
})

describe('SidebarProjectSwitcher', () => {
  it('names the active project and leaves it out of the icon list', () => {
    render(
      <SidebarProjectSwitcher
        projects={[project('p1', 'Alpha'), project('p2', 'Beta')]}
        activeProjectId="p1"
        onSelectProject={vi.fn()}
        onAddProject={vi.fn()}
      />,
    )
    const heading = screen.getByTestId('active-project-name')
    expect(heading).toHaveTextContent('Alpha')
    expect(screen.queryByRole('button', { name: 'Alpha' })).toBeNull()
    expect(screen.getByRole('button', { name: 'Beta' })).toBeInTheDocument()
  })

  it('lists every other project and selects one on click', async () => {
    const onSelectProject = vi.fn()
    render(
      <SidebarProjectSwitcher
        projects={[project('p1'), project('p2'), project('p3')]}
        activeProjectId="p1"
        onSelectProject={onSelectProject}
        onAddProject={vi.fn()}
      />,
    )
    expect(screen.getAllByTestId('space-mark')).toHaveLength(2)
    await userEvent.click(screen.getByRole('button', { name: 'p3' }))
    expect(onSelectProject).toHaveBeenCalledWith('p3')
  })

  it('lists every project when none is active', () => {
    render(
      <SidebarProjectSwitcher
        projects={[project('p1'), project('p2')]}
        onSelectProject={vi.fn()}
        onAddProject={vi.fn()}
      />,
    )
    expect(screen.queryByTestId('active-project-name')).toBeNull()
    expect(screen.getAllByTestId('space-mark')).toHaveLength(2)
  })

  it('the trailing + calls onAddProject', async () => {
    const onAddProject = vi.fn()
    render(
      <SidebarProjectSwitcher
        projects={[project('p1'), project('p2')]}
        activeProjectId="p1"
        onSelectProject={vi.fn()}
        onAddProject={onAddProject}
      />,
    )
    const row = screen.getByTestId('sidebar-project-switcher')
    const add = within(row).getByRole('button', { name: 'Add space' })
    expect(screen.getByRole('button', { name: 'p2' }).compareDocumentPosition(add)).toBe(
      Node.DOCUMENT_POSITION_FOLLOWING,
    )
    await userEvent.click(add)
    expect(onAddProject).toHaveBeenCalledOnce()
  })

  it('renders nothing with no projects and no add handler', () => {
    const { container } = render(<SidebarProjectSwitcher projects={[]} onSelectProject={vi.fn()} />)
    expect(container).toBeEmptyDOMElement()
  })
})
