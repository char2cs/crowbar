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

beforeEach(() => {
  sidebarPosition = 'left'
  toggleSidebar.mockClear()
  jump.handleJumpBack.mockClear()
  jump.handleJumpForward.mockClear()
  useJumpNavigation.mockClear()
})

describe('SidebarProjectHeader', () => {
  it('renders toggle, back, and forward', () => {
    render(<SidebarProjectHeader />)
    expect(screen.getByRole('button', { name: /sidebar/i })).toBeTruthy()
    expect(screen.getByRole('button', { name: /go back/i })).toBeTruthy()
    expect(screen.getByRole('button', { name: /go forward/i })).toBeTruthy()
  })

  it('toggles the sidebar', async () => {
    render(<SidebarProjectHeader />)
    await userEvent.click(screen.getByRole('button', { name: /sidebar/i }))
    expect(toggleSidebar).toHaveBeenCalledOnce()
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

  // Regression: this component is `memo`'d specifically because it takes
  // zero props, so a parent re-render (IDEShell, which re-renders on plenty
  // that has nothing to do with the sidebar) has nothing to compare that
  // could ever differ. `useJumpNavigation`'s own call count is a direct
  // proxy for how many times this component's function body actually ran —
  // if memoization regressed (e.g. the export stopped being wrapped in
  // `memo`), a parent re-render would call it again and this would catch it.
  it('does not re-render when an unrelated parent re-render happens (memoized, zero props)', async () => {
    function Harness() {
      const [, forceRerender] = useState(0)
      return (
        <div>
          <button onClick={() => forceRerender((n) => n + 1)}>bump</button>
          <SidebarProjectHeader />
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
