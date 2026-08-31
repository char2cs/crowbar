import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import type { Project } from '@/lib/types'

function makeProject(id: string): Project {
  return {
    id,
    name: id,
    path: `/repos/${id}`,
    lastActivity: new Date('2026-08-28T00:00:00Z'),
  }
}

const toggleSidebar = vi.fn()
vi.mock('@/components/ui/sidebar', () => ({
  useSidebar: () => ({ open: true, toggleSidebar }),
}))

const jump = {
  canGoBack: true,
  canGoForward: false,
  handleJumpBack: vi.fn(),
  handleJumpForward: vi.fn(),
}
vi.mock('@/features/tabs/hooks/use-jump-navigation', () => ({
  useJumpNavigation: () => jump,
}))

const openSettingsDialog = vi.fn()
vi.mock('@/features/window/stores/ui-state-store', () => ({
  useUIState: Object.assign(() => undefined, {
    getState: () => ({ openSettingsDialog }),
  }),
}))

let sidebarPosition: 'left' | 'right' = 'left'
vi.mock('@/features/settings/store', () => ({
  useSettingsStore: (sel: (s: unknown) => unknown) => sel({ settings: { sidebarPosition } }),
}))

import { SidebarProjectHeader } from '@/components/layout/sidebar-project-header'

beforeEach(() => {
  sidebarPosition = 'left'
  toggleSidebar.mockClear()
  openSettingsDialog.mockClear()
  jump.handleJumpBack.mockClear()
  jump.handleJumpForward.mockClear()
})

describe('SidebarProjectHeader', () => {
  it('renders toggle, back, forward, and settings', () => {
    render(<SidebarProjectHeader />)
    expect(screen.getByRole('button', { name: /sidebar/i })).toBeTruthy()
    expect(screen.getByRole('button', { name: /go back/i })).toBeTruthy()
    expect(screen.getByRole('button', { name: /go forward/i })).toBeTruthy()
    expect(screen.getByRole('button', { name: /settings/i })).toBeTruthy()
  })

  it('toggles the sidebar', async () => {
    render(<SidebarProjectHeader />)
    await userEvent.click(screen.getByRole('button', { name: /sidebar/i }))
    expect(toggleSidebar).toHaveBeenCalledOnce()
  })

  it('opens settings', async () => {
    render(<SidebarProjectHeader />)
    await userEvent.click(screen.getByRole('button', { name: /settings/i }))
    expect(openSettingsDialog).toHaveBeenCalledOnce()
  })

  it('disables forward when canGoForward is false and runs back when enabled', async () => {
    render(<SidebarProjectHeader />)
    expect(screen.getByRole('button', { name: /go forward/i })).toBeDisabled()
    await userEvent.click(screen.getByRole('button', { name: /go back/i }))
    expect(jump.handleJumpBack).toHaveBeenCalledOnce()
  })

  it('mirrors the layout when the sidebar is on the right', () => {
    sidebarPosition = 'right'
    const { container } = render(<SidebarProjectHeader />)
    const root = container.firstChild as HTMLElement
    expect(root.className).toContain('flex-row-reverse')
    // Traffic-light spacer is only reserved when the sidebar is on the left.
    expect(container.querySelector('.w-\\[52px\\]')).toBeNull()
  })

  it('renders one icon-only mark per project in the chrome middle', () => {
    const projects = [makeProject('p1'), makeProject('p2')]
    render(
      <SidebarProjectHeader projects={projects} activeProjectId="p1" onSelectProject={vi.fn()} />,
    )
    expect(screen.getAllByTestId('space-mark')).toHaveLength(2)
  })

  it('the current space mark is full strength, others muted', () => {
    const projects = [makeProject('p1'), makeProject('p2')]
    render(
      <SidebarProjectHeader projects={projects} activeProjectId="p1" onSelectProject={vi.fn()} />,
    )
    const marks = screen.getAllByTestId('space-mark')
    expect(marks[0]).not.toHaveClass('opacity-60')
    expect(marks[1]).toHaveClass('opacity-60')
  })

  it('clicking a mark calls onSelectProject with that project id', async () => {
    const projects = [makeProject('p1'), makeProject('p2')]
    const onSelectProject = vi.fn()
    render(
      <SidebarProjectHeader
        projects={projects}
        activeProjectId="p1"
        onSelectProject={onSelectProject}
      />,
    )
    const marks = screen.getAllByTestId('space-mark')
    await userEvent.click(marks[1])
    expect(onSelectProject).toHaveBeenCalledWith('p2')
  })

  it('renders no marks and behaves as the bare spacer when no projects are given', () => {
    render(<SidebarProjectHeader />)
    expect(screen.queryAllByTestId('space-mark')).toHaveLength(0)
  })

  it('renders a trailing add-project mark after the last project mark when onAddProject is supplied', () => {
    const projects = [makeProject('p1'), makeProject('p2')]
    render(
      <SidebarProjectHeader
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
      <SidebarProjectHeader
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
    render(<SidebarProjectHeader projects={[makeProject('p1')]} activeProjectId="p1" />)
    expect(screen.queryByTestId('add-project-mark')).not.toBeInTheDocument()
  })
})
