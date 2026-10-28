import { render, screen, waitFor } from '@testing-library/react'
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
import { pairRemoteHost, resetHostWorld } from '@/__tests__/__fixtures__/remote-host'

beforeEach(() => {
  resetHostWorld()
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

describe('SidebarProjectHeader host filter', () => {
  const hostIds = ['local', 'h1', 'h2']

  beforeEach(() => {
    pairRemoteHost('h1', 'Build box')
    pairRemoteHost('h2', 'Laptop')
  })

  it('has no host control with only the local machine', () => {
    render(<SidebarProjectHeader hostIds={['local']} shownHost="all" onSelectHost={vi.fn()} />)
    expect(screen.queryByRole('button', { name: /^host/i })).toBeNull()
    expect(screen.getAllByRole('button')).toHaveLength(2)
  })

  it('names what it shows on the control, between Settings and the toggle', () => {
    render(<SidebarProjectHeader hostIds={hostIds} shownHost="all" onSelectHost={vi.fn()} />)
    const labels = screen.getAllByRole('button').map((b) => b.getAttribute('aria-label'))
    expect(labels).toEqual(['Settings', 'Host: All hosts', 'Hide sidebar'])
  })

  it('names the chosen host on the control', () => {
    render(<SidebarProjectHeader hostIds={hostIds} shownHost="h1" onSelectHost={vi.fn()} />)
    expect(screen.getByRole('button', { name: 'Host: Build box' })).toHaveTextContent('Build box')
  })

  it('lists all hosts, the local machine and every paired host with its state', async () => {
    const user = userEvent.setup()
    render(<SidebarProjectHeader hostIds={hostIds} shownHost="h1" onSelectHost={vi.fn()} />)
    await user.click(screen.getByRole('button', { name: 'Host: Build box' }))
    const items = await screen.findAllByRole('menuitemradio')
    expect(items.map((i) => i.textContent)).toEqual([
      'All hosts',
      expect.stringContaining('Local'),
      expect.stringContaining('Build box'),
      expect.stringContaining('Laptop'),
    ])
    expect(await screen.findByRole('menuitemradio', { name: /Build box/ })).toBeChecked()
    expect(screen.getByTestId('host-status-h2')).toBeInTheDocument()
  })

  it('reports the host chosen', async () => {
    const user = userEvent.setup()
    const onSelectHost = vi.fn()
    render(<SidebarProjectHeader hostIds={hostIds} shownHost="all" onSelectHost={onSelectHost} />)
    await user.click(screen.getByRole('button', { name: 'Host: All hosts' }))
    await user.click(await screen.findByRole('menuitemradio', { name: /Laptop/ }))
    expect(onSelectHost).toHaveBeenCalledExactlyOnceWith('h2')
    await waitFor(() => expect(screen.queryByRole('menu')).toBeNull())
  })

  it('reports all hosts chosen', async () => {
    const user = userEvent.setup()
    const onSelectHost = vi.fn()
    render(<SidebarProjectHeader hostIds={hostIds} shownHost="h1" onSelectHost={onSelectHost} />)
    await user.click(screen.getByRole('button', { name: 'Host: Build box' }))
    await user.click(await screen.findByRole('menuitemradio', { name: 'All hosts' }))
    expect(onSelectHost).toHaveBeenCalledExactlyOnceWith('all')
  })
})
