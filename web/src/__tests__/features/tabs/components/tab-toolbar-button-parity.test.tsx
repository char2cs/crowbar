import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import CloseSplitButton from '@/features/tabs/components/close-split-button'
import TabAddButton from '@/features/tabs/components/tab-add-button'
import TabNavigationButtons from '@/features/tabs/components/tab-navigation-buttons'
import { SidebarProjectHeader } from '@/components/layout/sidebar-project-header'

// SidebarProjectHeader reads useSidebar() only for the toggle's label and click
// handler — neither affects the styling under test — so stub it rather than
// stand up a SidebarProvider.
vi.mock('@/components/ui/sidebar', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/components/ui/sidebar')>()),
  useSidebar: () => ({ open: true, toggleSidebar: () => {} }),
}))

// The sidebar toggle exists twice: in SidebarProjectHeader while the sidebar is
// open, and in the tab bar once it is hidden. They occupy the same corner, so
// any divergence in size/radius/hover reads as the button changing shape when
// you collapse the sidebar — which is exactly what happened when the tab-bar
// copy was left on `icon-xs` (24px/8px) beside `icon-sm` peers (28px/6px).
//
// Compare the full class list rather than spot-checking a token: the drift was
// three separate properties, and only whole-recipe equality catches the next one.
const classesOf = (el: HTMLElement) => new Set(el.className.split(/\s+/).filter(Boolean))

// The mirror is the one legitimate difference — both toggles flip when the
// sidebar is docked right, and only the toggles carry it.
const MIRROR = 'scale-x-[-1]'
const withoutMirror = (el: HTMLElement) => {
  const classes = classesOf(el)
  classes.delete(MIRROR)
  return classes
}

const diff = (a: Set<string>, b: Set<string>) => [
  [...a].filter((c) => !b.has(c)),
  [...b].filter((c) => !a.has(c)),
]

describe('tab bar toolbar button parity', () => {
  it('renders the sidebar toggle with the same recipe as its tab-bar peers', () => {
    render(
      <>
        <TabNavigationButtons
          isBottomPane={false}
          sidebarOpen={false}
          sidebarPosition="left"
          onToggleSidebar={() => {}}
        />
        <TabAddButton isBottomPane={false} onNewTab={() => {}} />
        <CloseSplitButton
          isBottomPane={false}
          disablePaneActions={false}
          isInSplit
          onClosePane={() => {}}
        />
      </>,
    )

    const toggle = withoutMirror(screen.getByRole('button', { name: 'Show sidebar' }))
    const addTab = classesOf(screen.getByRole('button', { name: 'New tab' }))
    const closeSplit = classesOf(screen.getByRole('button', { name: 'Close split pane' }))

    expect(diff(toggle, addTab)).toEqual([[], []])
    expect(diff(toggle, closeSplit)).toEqual([[], []])
  })

  it('renders the same toggle whether the sidebar is hidden or shown', () => {
    const { unmount } = render(
      <TabNavigationButtons
        isBottomPane={false}
        sidebarOpen={false}
        sidebarPosition="left"
        onToggleSidebar={() => {}}
      />,
    )
    const hidden = withoutMirror(screen.getByRole('button', { name: 'Show sidebar' }))
    unmount()

    render(<SidebarProjectHeader />)
    const shown = withoutMirror(screen.getByRole('button', { name: /sidebar$/i }))

    expect(diff(hidden, shown)).toEqual([[], []])
  })

  it('mirrors only when the sidebar is docked right', () => {
    const { unmount } = render(
      <TabNavigationButtons
        isBottomPane={false}
        sidebarOpen={false}
        sidebarPosition="left"
        onToggleSidebar={() => {}}
      />,
    )
    expect(classesOf(screen.getByRole('button', { name: 'Show sidebar' })).has(MIRROR)).toBe(false)
    unmount()

    render(
      <TabNavigationButtons
        isBottomPane={false}
        sidebarOpen={false}
        sidebarPosition="right"
        onToggleSidebar={() => {}}
      />,
    )
    expect(classesOf(screen.getByRole('button', { name: 'Show sidebar' })).has(MIRROR)).toBe(true)
  })
})
