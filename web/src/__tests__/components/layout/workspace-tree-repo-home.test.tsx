import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen } from '@testing-library/react'

// Router hooks used by WorkspaceTreeInner — stub them so the component renders
// in isolation (mirrors workspace-tree-error.test.tsx).
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => () => {},
  useRouterState: () => '',
  useRouter: () => ({ state: { location: { pathname: '/' } } }),
  useMatch: () => null,
}))

import { idle } from '@/lib/loadable'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { useHomeWorkspaceStore } from '@/lib/store/home-workspace'
import { WorkspaceTree } from '@/components/layout/workspace-tree'

const repo = (over: Partial<Repo> = {}): Repo => ({
  id: 'r1',
  projectId: 'p1',
  name: 'crowbar',
  avatarLabel: 'C',
  avatarColor: 'bg-indigo-700',
  defaultWorkspaceId: 'w-default',
  workspaces: [],
  ...over,
})

beforeEach(() => {
  useWorkspaceListStore.setState({ data: idle() })
  // Keep project home idle so the only spinner that can appear is the repo
  // header's — the assertions below stay unambiguous.
  useHomeWorkspaceStore.setState({ workspace: null })
})

// The repo header row IS the repo-home (default) workspace's tile. An agent
// working in it must move that icon into its loading state, exactly as a
// worktree row's branch glyph becomes the spinner.
describe('WorkspaceTree repo-home working overlay', () => {
  it('spins the repo header icon while the repo-home workspace is working', () => {
    useSidebarStore.setState({ repos: [repo({ defaultWorking: true })] })

    const { container } = render(<WorkspaceTree />)

    expect(screen.getByRole('status', { name: 'Loading' })).toBeInTheDocument()
    // The real flicker spinner (self-animating SVG), theme-token colored.
    expect(container.querySelector('svg animate')).not.toBeNull()
    expect(container.querySelector('.text-primary')).not.toBeNull()
    // The avatar initials yield to the spinner for the duration of the turn.
    expect(screen.queryByText('C')).toBeNull()
  })

  it('shows the repo avatar (no spinner) when the repo home is idle', () => {
    useSidebarStore.setState({ repos: [repo({ defaultWorking: false })] })

    render(<WorkspaceTree />)

    expect(screen.queryByRole('status', { name: 'Loading' })).toBeNull()
    expect(screen.getByText('C')).toBeInTheDocument()
  })

  it('does not regress a worktree row: its own working flag still spins', () => {
    useSidebarStore.setState({
      repos: [
        repo({
          defaultWorking: false,
          workspaces: [{ id: 'ws1', branch: 'feature/x', status: 'new', age: '1d', working: true }],
        }),
      ],
    })

    render(<WorkspaceTree />)

    // The worktree row spins; the idle repo header still shows its avatar.
    expect(screen.getByRole('status', { name: 'Loading' })).toBeInTheDocument()
    expect(screen.getByText('C')).toBeInTheDocument()
  })
})
