import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'

// Router hooks used by WorkspaceTreeInner — stub them so the component renders
// in isolation (mirrors workspace-tree-error.test.tsx).
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => () => {},
  useRouterState: () => '',
  useRouter: () => ({ state: { location: { pathname: '/' } } }),
  useMatch: () => null,
}))

// Mock only renameRepo — the repo row's inline rename fires it on confirm; every
// other api export stays real.
vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return { ...actual, renameRepo: vi.fn(() => Promise.resolve()) }
})

import { idle } from '@/lib/loadable'
import * as api from '@/lib/api'
import { toast } from '@/features/window/stores/toast-store'
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
  vi.mocked(api.renameRepo).mockClear()
})

// The repo name is renamed inline off its own row — double-click swaps the name
// for a text editor, exactly like a branch row (spec: mirror the branch rename).
describe('WorkspaceTree repo inline rename', () => {
  it('double-clicking the repo name renames it via renameRepo(projectId, repoId, name)', () => {
    useSidebarStore.setState({ repos: [repo({ name: 'crowbar' })] })
    render(<WorkspaceTree />)

    fireEvent.doubleClick(screen.getByText('crowbar'))

    const input = screen.getByDisplayValue('crowbar') as HTMLInputElement
    fireEvent.change(input, { target: { value: 'renamed-repo' } })
    fireEvent.keyDown(input, { key: 'Enter' })

    expect(vi.mocked(api.renameRepo)).toHaveBeenCalledWith('p1', 'r1', 'renamed-repo')
  })

  it('Escape cancels the inline rename without calling renameRepo', () => {
    useSidebarStore.setState({ repos: [repo({ name: 'crowbar' })] })
    render(<WorkspaceTree />)

    fireEvent.doubleClick(screen.getByText('crowbar'))
    const input = screen.getByDisplayValue('crowbar') as HTMLInputElement
    fireEvent.change(input, { target: { value: 'discarded' } })
    fireEvent.keyDown(input, { key: 'Escape' })

    expect(vi.mocked(api.renameRepo)).not.toHaveBeenCalled()
    expect(screen.getByText('crowbar')).toBeInTheDocument()
  })

  it('surfaces a toast when the rename request fails', async () => {
    const spy = vi.spyOn(toast, 'error').mockImplementation(() => '')
    vi.mocked(api.renameRepo).mockRejectedValueOnce(new Error('name taken'))
    useSidebarStore.setState({ repos: [repo({ name: 'crowbar' })] })
    render(<WorkspaceTree />)

    fireEvent.doubleClick(screen.getByText('crowbar'))
    const input = screen.getByDisplayValue('crowbar') as HTMLInputElement
    fireEvent.change(input, { target: { value: 'renamed' } })
    fireEvent.keyDown(input, { key: 'Enter' })

    await waitFor(() => expect(spy).toHaveBeenCalled())
    spy.mockRestore()
  })
})

// The repo header row IS the repo-home (default) workspace's tile. An agent
// working in it must move that icon into its loading state, exactly as a
// worktree row's branch glyph becomes the spinner.
describe('WorkspaceTree repo-home working overlay', () => {
  it('spins the repo header icon while the repo-home workspace is working', () => {
    useSidebarStore.setState({ repos: [repo({ defaultWorking: true })] })

    const { container } = render(<WorkspaceTree />)

    expect(screen.getByRole('status', { name: 'Loading' })).toBeInTheDocument()
    // The real flicker spinner, theme-token colored.
    expect(container.querySelector('[data-flicker-spinner]')).not.toBeNull()
    expect(container.querySelector('.text-foreground')).not.toBeNull()
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
