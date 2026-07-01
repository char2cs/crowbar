import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen } from '@testing-library/react'

// Router hooks used by WorkspaceTreeInner — stub them so the component renders in isolation.
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => () => {},
  useRouterState: () => '',
  useRouter: () => ({ state: { location: { pathname: '/' } } }),
  useMatch: () => null,
}))

import { failed, idle } from '@/lib/loadable'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { useSidebarStore } from '@/lib/store/sidebar'
import { WorkspaceTree } from '@/components/layout/workspace-tree'

beforeEach(() => {
  useSidebarStore.setState({ repos: [] })
  useWorkspaceListStore.setState({ data: idle() })
})

describe('WorkspaceTree error state', () => {
  it('shows inline error when workspaces failed to load and there are no repos', () => {
    useWorkspaceListStore.setState({ data: failed(new Error('500'), idle()) })
    render(<WorkspaceTree />)
    expect(screen.getByText(/failed to load/i)).toBeInTheDocument()
  })

  it('does not show error when repos are present (cached)', () => {
    useSidebarStore.setState({ repos: [{ id: 'r1', name: 'repo', workspaces: [] }] as never })
    useWorkspaceListStore.setState({ data: failed(new Error('500'), idle()) })
    render(<WorkspaceTree />)
    expect(screen.queryByText(/failed to load/i)).toBeNull()
  })
})
