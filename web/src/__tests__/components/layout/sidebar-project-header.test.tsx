import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, beforeEach, vi } from 'vitest'

const toggleSidebar = vi.fn()
vi.mock('@/components/ui/sidebar', () => ({
  useSidebar: () => ({ open: true, toggleSidebar }),
}))

let sidebarPosition: 'left' | 'right' = 'left'
vi.mock('@/features/settings/store', () => ({
  useSettingsStore: (sel: (s: unknown) => unknown) => sel({ settings: { sidebarPosition } }),
}))

import { SidebarProjectHeader } from '@/components/layout/sidebar-project-header'
import type { Project } from '@/lib/types'

const project = (id: string): Project => ({
  id,
  name: id,
  path: `/repos/${id}`,
  lastActivity: new Date('2026-08-28T00:00:00Z'),
})
const headerProps = {
  projects: [project('p1'), project('p2')],
  activeProjectId: 'p1',
  onSelectProject: vi.fn(),
  onAddProject: vi.fn(),
}

beforeEach(() => {
  sidebarPosition = 'left'
  toggleSidebar.mockClear()
})

describe('SidebarProjectHeader', () => {
  it('renders the sidebar toggle and no history arrows', () => {
    render(<SidebarProjectHeader {...headerProps} />)
    expect(screen.getByRole('button', { name: /sidebar/i })).toBeTruthy()
    expect(screen.queryByRole('button', { name: /go back/i })).toBeNull()
    expect(screen.queryByRole('button', { name: /go forward/i })).toBeNull()
  })

  it('toggles the sidebar', async () => {
    render(<SidebarProjectHeader {...headerProps} />)
    await userEvent.click(screen.getByRole('button', { name: /sidebar/i }))
    expect(toggleSidebar).toHaveBeenCalledOnce()
  })

  it('mirrors the layout when the sidebar is on the right', () => {
    sidebarPosition = 'right'
    const { container } = render(<SidebarProjectHeader {...headerProps} />)
    const root = container.firstChild as HTMLElement
    expect(root.className).toContain('flex-row-reverse')
    // Traffic-light spacer is only reserved when the sidebar is on the left.
    expect(container.querySelector('.w-\\[52px\\]')).toBeNull()
  })

  it('renders the space marks in the window-chrome row and forwards selection', async () => {
    const onSelectProject = vi.fn()
    render(<SidebarProjectHeader {...headerProps} onSelectProject={onSelectProject} />)
    const marks = screen.getAllByTestId('space-mark')
    expect(marks).toHaveLength(2)
    await userEvent.click(marks[1])
    expect(onSelectProject).toHaveBeenCalledWith('p2')
  })
})
