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
import { useUIState } from '@/features/window/stores/ui-state-store'

beforeEach(() => {
  sidebarPosition = 'left'
  toggleSidebar.mockClear()
})

describe('SidebarProjectHeader', () => {
  it('renders the sidebar toggle and no history arrows', () => {
    render(<SidebarProjectHeader />)
    expect(screen.getByRole('button', { name: /sidebar/i })).toBeTruthy()
    expect(screen.queryByRole('button', { name: /go back/i })).toBeNull()
    expect(screen.queryByRole('button', { name: /go forward/i })).toBeNull()
  })

  it('toggles the sidebar', async () => {
    render(<SidebarProjectHeader />)
    await userEvent.click(screen.getByRole('button', { name: /sidebar/i }))
    expect(toggleSidebar).toHaveBeenCalledOnce()
  })

  it('mirrors the layout when the sidebar is on the right', () => {
    sidebarPosition = 'right'
    const { container } = render(<SidebarProjectHeader />)
    const root = container.firstChild as HTMLElement
    expect(root.className).toContain('flex-row-reverse')
    // Traffic-light spacer is only reserved when the sidebar is on the left.
    expect(container.querySelector('.w-\\[52px\\]')).toBeNull()
  })

  it('holds exactly the sidebar toggle and Settings: no marks, no build readout', () => {
    render(<SidebarProjectHeader />)
    expect(screen.getAllByRole('button').map((b) => b.getAttribute('aria-label'))).toEqual([
      'Settings',
      'Hide sidebar',
    ])
    expect(screen.queryByTestId('space-mark')).toBeNull()
    expect(screen.queryByText(/fps/)).toBeNull()
  })

  it('Settings opens the settings dialog', async () => {
    useUIState.setState({ isSettingsOpen: false })
    render(<SidebarProjectHeader />)
    await userEvent.click(screen.getByRole('button', { name: 'Settings' }))
    expect(useUIState.getState().isSettingsOpen).toBe(true)
  })
})
