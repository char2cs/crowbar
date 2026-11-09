import { render, screen, waitFor, within } from '@testing-library/react'
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

describe('SidebarProjectHeader host picker', () => {
  const hostIds = ['local', 'h1', 'h2']

  beforeEach(() => {
    pairRemoteHost('h1', 'Build box')
    pairRemoteHost('h2', 'Laptop')
  })

  it('has no host control while there is no host to choose', () => {
    render(<SidebarProjectHeader hostIds={['local']} onSelectHost={vi.fn()} />)
    expect(screen.queryByRole('button', { name: /^host/i })).toBeNull()
    expect(screen.getAllByRole('button')).toHaveLength(2)
  })

  it('names the shown host on the control, between Settings and the toggle', () => {
    render(<SidebarProjectHeader hostIds={hostIds} shownHost="h1" onSelectHost={vi.fn()} />)
    const labels = screen.getAllByRole('button').map((b) => b.getAttribute('aria-label'))
    expect(labels).toEqual(['Settings', 'Host: Build box', 'Hide sidebar'])
    expect(screen.getByRole('button', { name: 'Host: Build box' })).toHaveTextContent('Build box')
  })

  it('says the shown host’s state in words', () => {
    render(<SidebarProjectHeader hostIds={hostIds} shownHost="h1" onSelectHost={vi.fn()} />)
    const picker = screen.getByRole('button', { name: 'Host: Build box' })
    expect(within(picker).getByRole('img')).toHaveAccessibleName('Status unknown')
  })

  it('lists the local machine and every paired host, marking the shown one', async () => {
    const user = userEvent.setup()
    render(<SidebarProjectHeader hostIds={hostIds} shownHost="h1" onSelectHost={vi.fn()} />)
    await user.click(screen.getByRole('button', { name: 'Host: Build box' }))
    const items = await screen.findAllByRole('menuitem')
    expect(items.map((i) => i.textContent)).toEqual([
      'LocalThis machine',
      'Build boxh1.example:3737Current',
      'Laptoph2.example:3737',
    ])
    expect(screen.getByRole('menuitem', { name: /Build box/ })).toHaveAttribute(
      'aria-current',
      'true',
    )
    expect(screen.getByTestId('host-status-h2')).toBeInTheDocument()
  })

  it('reports the host chosen and closes', async () => {
    const user = userEvent.setup()
    const onSelectHost = vi.fn()
    render(<SidebarProjectHeader hostIds={hostIds} shownHost="h1" onSelectHost={onSelectHost} />)
    await user.click(screen.getByRole('button', { name: 'Host: Build box' }))
    await user.click(await screen.findByRole('menuitem', { name: /Laptop/ }))
    expect(onSelectHost).toHaveBeenCalledExactlyOnceWith('h2')
    await waitFor(() => expect(screen.queryByRole('menu')).toBeNull())
  })
})
