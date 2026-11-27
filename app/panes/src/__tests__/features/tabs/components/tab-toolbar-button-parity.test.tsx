import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { BranchReviewShortcutButton } from '@/features/tabs/components/branch-review-shortcut-button'
import TabNavigationButtons from '@/features/tabs/components/tab-navigation-buttons'

const classesOf = (el: HTMLElement) => new Set(el.className.split(/\s+/).filter(Boolean))

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
        <BranchReviewShortcutButton isBottomPane={false} onOpen={() => {}} />
      </>,
    )

    const toggle = withoutMirror(screen.getByRole('button', { name: 'Show sidebar' }))
    const branchReview = classesOf(screen.getByRole('button', { name: 'Review this branch' }))

    expect(diff(toggle, branchReview)).toEqual([[], []])
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
