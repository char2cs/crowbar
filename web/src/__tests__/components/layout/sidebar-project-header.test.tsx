import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, beforeEach, vi } from 'vitest'

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

  // task-10 (sidebar-restyle-recovery-batch2): the project-marks cluster
  // moved out of this window-chrome row entirely, to SidebarFooter — the
  // sidebar's own true last element, below the floating file-explorer card.
  // This component takes no project-related props any more; it degrades to
  // exactly its pre-marks shape (a bare `flex-1` spacer between the toggle
  // and the back/forward/settings cluster).
  it('renders no project marks in the window-chrome row', () => {
    render(<SidebarProjectHeader />)
    expect(screen.queryAllByTestId('space-mark')).toHaveLength(0)
    expect(screen.queryByTestId('add-project-mark')).not.toBeInTheDocument()
  })
})
