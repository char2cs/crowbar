import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const useMatch = vi.fn()
vi.mock('@tanstack/react-router', () => ({ useMatch: (...a: unknown[]) => useMatch(...a) }))

import { SidebarTabBar } from '@/components/layout/sidebar-tab-bar'
import { useSidebarStore } from '@/lib/store/sidebar'

describe('SidebarTabBar Chats tab', () => {
  beforeEach(() => useSidebarStore.setState(useSidebarStore.getInitialState()))

  it('shows Chats as the 2nd tab on a non-home route (worktree/repo-home route shape)', () => {
    useMatch.mockReturnValue(null) // not a home route
    render(<SidebarTabBar />)
    expect(screen.getByText('Chats')).toBeTruthy()
    expect(screen.getByText('Git')).toBeTruthy()
  })

  it('shows Chats on a project-home route (only Git is filtered)', () => {
    useMatch.mockReturnValue({}) // home route match
    render(<SidebarTabBar />)
    expect(screen.getByText('Chats')).toBeTruthy()
    expect(screen.queryByText('Git')).toBeNull()
  })
})
