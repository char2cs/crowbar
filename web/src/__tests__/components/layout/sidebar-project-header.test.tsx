import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { useState } from 'react'

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
// A spy on the HOOK ITSELF (not just what it returns) — counting calls to
// this is counting how many times SidebarProjectHeader's function body ran,
// which is exactly what the memoization test below needs to observe.
const useJumpNavigation = vi.fn(() => jump)
vi.mock('@/features/tabs/hooks/use-jump-navigation', () => ({
  useJumpNavigation: () => useJumpNavigation(),
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
  jump.handleJumpBack.mockClear()
  jump.handleJumpForward.mockClear()
  useJumpNavigation.mockClear()
})

describe('SidebarProjectHeader', () => {
  it('renders toggle, back, and forward', () => {
    render(<SidebarProjectHeader {...headerProps} />)
    expect(screen.getByRole('button', { name: /sidebar/i })).toBeTruthy()
    expect(screen.getByRole('button', { name: /go back/i })).toBeTruthy()
    expect(screen.getByRole('button', { name: /go forward/i })).toBeTruthy()
  })

  it('toggles the sidebar', async () => {
    render(<SidebarProjectHeader {...headerProps} />)
    await userEvent.click(screen.getByRole('button', { name: /sidebar/i }))
    expect(toggleSidebar).toHaveBeenCalledOnce()
  })

  it('disables forward when canGoForward is false and runs back when enabled', async () => {
    render(<SidebarProjectHeader {...headerProps} />)
    expect(screen.getByRole('button', { name: /go forward/i })).toBeDisabled()
    await userEvent.click(screen.getByRole('button', { name: /go back/i }))
    expect(jump.handleJumpBack).toHaveBeenCalledOnce()
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

  // Regression: this component is `memo`'d specifically because it takes
  // zero props, so a parent re-render (IDEShell, which re-renders on plenty
  // that has nothing to do with the sidebar) has nothing to compare that
  // could ever differ. `useJumpNavigation`'s own call count is a direct
  // proxy for how many times this component's function body actually ran —
  // if memoization regressed (e.g. the export stopped being wrapped in
  // `memo`), a parent re-render would call it again and this would catch it.
  it('does not re-render when an unrelated parent re-render happens (memoized)', async () => {
    function Harness() {
      const [, forceRerender] = useState(0)
      return (
        <div>
          <button onClick={() => forceRerender((n) => n + 1)}>bump</button>
          <SidebarProjectHeader {...headerProps} />
        </div>
      )
    }
    render(<Harness />)
    const callsAfterMount = useJumpNavigation.mock.calls.length
    expect(callsAfterMount).toBeGreaterThan(0)

    await userEvent.click(screen.getByRole('button', { name: 'bump' }))

    expect(useJumpNavigation.mock.calls.length).toBe(callsAfterMount)
  })
})
