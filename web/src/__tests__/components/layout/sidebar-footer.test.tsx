import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, beforeEach } from 'vitest'
import { vi } from 'vitest'
import type { Project } from '@/lib/types'

let sidebarPosition: 'left' | 'right' = 'left'
vi.mock('@/features/settings/store', () => ({
  useSettingsStore: (sel: (s: unknown) => unknown) => sel({ settings: { sidebarPosition } }),
}))

const openSettingsDialog = vi.fn()
vi.mock('@/features/window/stores/ui-state-store', () => ({
  useUIState: Object.assign(() => undefined, {
    getState: () => ({ openSettingsDialog }),
  }),
}))

import { SidebarFooter } from '@/components/layout/sidebar-footer'

beforeEach(() => {
  sidebarPosition = 'left'
  openSettingsDialog.mockClear()
})

function makeProject(id: string): Project {
  return {
    id,
    name: id,
    path: `/repos/${id}`,
    lastActivity: new Date('2026-08-28T00:00:00Z'),
  }
}

describe('SidebarFooter', () => {
  it('renders one icon-only mark per project', () => {
    const projects = [makeProject('p1'), makeProject('p2')]
    render(<SidebarFooter projects={projects} activeProjectId="p1" onSelectProject={vi.fn()} />)
    expect(screen.getAllByTestId('space-mark')).toHaveLength(2)
  })

  it('the current space mark is full strength, others muted', () => {
    const projects = [makeProject('p1'), makeProject('p2')]
    render(<SidebarFooter projects={projects} activeProjectId="p1" onSelectProject={vi.fn()} />)
    const marks = screen.getAllByTestId('space-mark')
    expect(marks[0]).not.toHaveClass('opacity-60')
    expect(marks[1]).toHaveClass('opacity-60')
  })

  it('a muted mark previews full color on hover, before it becomes current', () => {
    const projects = [makeProject('p1'), makeProject('p2')]
    render(<SidebarFooter projects={projects} activeProjectId="p1" onSelectProject={vi.fn()} />)
    const marks = screen.getAllByTestId('space-mark')
    expect(marks[1]).toHaveClass('hover:opacity-100')
  })

  it('clicking a mark calls onSelectProject with that project id', async () => {
    const projects = [makeProject('p1'), makeProject('p2')]
    const onSelectProject = vi.fn()
    render(
      <SidebarFooter projects={projects} activeProjectId="p1" onSelectProject={onSelectProject} />,
    )
    const marks = screen.getAllByTestId('space-mark')
    await userEvent.click(marks[1])
    expect(onSelectProject).toHaveBeenCalledWith('p2')
  })

  it('renders no marks when no projects are given', () => {
    render(<SidebarFooter onAddProject={vi.fn()} />)
    expect(screen.queryAllByTestId('space-mark')).toHaveLength(0)
  })

  it('renders a trailing add-project mark after the last project mark when onAddProject is supplied', () => {
    const projects = [makeProject('p1'), makeProject('p2')]
    render(
      <SidebarFooter
        projects={projects}
        activeProjectId="p1"
        onSelectProject={vi.fn()}
        onAddProject={vi.fn()}
      />,
    )
    const marks = screen.getAllByTestId('space-mark')
    const addMark = screen.getByTestId('add-project-mark')
    // Trailing: it comes after every project mark in document order.
    expect(marks[marks.length - 1].compareDocumentPosition(addMark)).toBe(
      Node.DOCUMENT_POSITION_FOLLOWING,
    )
  })

  it('clicking the add-project mark calls onAddProject', async () => {
    const onAddProject = vi.fn()
    render(
      <SidebarFooter
        projects={[makeProject('p1')]}
        activeProjectId="p1"
        onSelectProject={vi.fn()}
        onAddProject={onAddProject}
      />,
    )
    await userEvent.click(screen.getByTestId('add-project-mark'))
    expect(onAddProject).toHaveBeenCalledOnce()
  })

  it('omits the add-project mark entirely when onAddProject is not supplied', () => {
    render(<SidebarFooter projects={[makeProject('p1')]} activeProjectId="p1" />)
    expect(screen.queryByTestId('add-project-mark')).not.toBeInTheDocument()
  })

  it('renders nothing at all when there are no projects and no onAddProject', () => {
    const { container } = render(<SidebarFooter />)
    expect(container).toBeEmptyDOMElement()
  })

  it('renders as a self-contained footer element (data-testid=sidebar-footer)', () => {
    render(<SidebarFooter onAddProject={vi.fn()} />)
    expect(screen.getByTestId('sidebar-footer')).toBeInTheDocument()
  })

  it('opens settings when the settings button is clicked', async () => {
    render(<SidebarFooter onAddProject={vi.fn()} />)
    await userEvent.click(screen.getByTestId('settings-button'))
    expect(openSettingsDialog).toHaveBeenCalledOnce()
  })

  it('pins settings to the outer window edge: left when the sidebar is left', () => {
    sidebarPosition = 'left'
    render(<SidebarFooter onAddProject={vi.fn()} />)
    expect(screen.getByTestId('settings-button')).toHaveClass('left-2')
  })

  it('pins settings to the outer window edge: right when the sidebar is right', () => {
    sidebarPosition = 'right'
    render(<SidebarFooter onAddProject={vi.fn()} />)
    expect(screen.getByTestId('settings-button')).toHaveClass('right-2')
  })

  it('pins + to the content-facing edge, opposite settings: right when the sidebar is left', () => {
    sidebarPosition = 'left'
    render(<SidebarFooter onAddProject={vi.fn()} />)
    expect(screen.getByTestId('add-project-mark')).toHaveClass('right-2')
  })

  it('pins + to the content-facing edge, opposite settings: left when the sidebar is right', () => {
    sidebarPosition = 'right'
    render(<SidebarFooter onAddProject={vi.fn()} />)
    expect(screen.getByTestId('add-project-mark')).toHaveClass('left-2')
  })
})
