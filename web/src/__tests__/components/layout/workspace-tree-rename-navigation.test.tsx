/**
 * The repo header row's NAME answers two gestures: one click opens the
 * repo-home workspace, two clicks rename the repo. `dblclick` is delivered only
 * after BOTH of its `click` events have already fired, so the two cannot be told
 * apart at the first click.
 *
 * This row resolves that WITHOUT a timer: the click is never held, so opening
 * the workspace is instant, and a rename simply navigates into the repo home on
 * its way to the editor. An earlier version deferred every name click for a
 * double-click window, which put a visible stall in front of the sidebar's
 * most-used gesture. An earlier one still swallowed the click outright, which
 * made the biggest target on the row look dead. These tests pin the middle
 * ground: no delay, no dead click, and the rename editor still opens.
 *
 * No fake timers anywhere in this file — that absence IS the contract.
 */
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const navigateFn = vi.hoisted(() => vi.fn())

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => navigateFn,
  useRouterState: () => '',
  useRouter: () => ({ state: { location: { pathname: '/' } } }),
  useMatch: () => null,
}))

vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return { ...actual, renameRepo: vi.fn(() => Promise.resolve()) }
})

vi.mock('@/lib/api/sidebar-placement', () => ({
  placeWorkspace: vi.fn(() => Promise.resolve()),
  placeFolder: vi.fn(() => Promise.resolve()),
  placeRepo: vi.fn(() => Promise.resolve()),
  placeProject: vi.fn(() => Promise.resolve()),
  createFolder: vi.fn(() => Promise.resolve({ id: 'new-folder' })),
  deleteFolder: vi.fn(() => Promise.resolve()),
}))

import { WorkspaceTree } from '@/components/layout/workspace-tree'
import { placeFolder } from '@/lib/api/sidebar-placement'
import { idle } from '@/lib/loadable'
import { useHomeWorkspaceStore } from '@/lib/store/home-workspace'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'

const repo = (over: Partial<Repo> = {}): Repo => ({
  id: 'r1',
  projectId: 'p1',
  name: 'crowbar',
  avatarLabel: 'C',
  avatarColor: 'bg-indigo-700',
  defaultWorkspaceId: 'w-default',
  workspaces: [{ id: 'ws1', branch: 'feature/x', status: 'new', age: '1d' }],
  ...over,
})

beforeEach(() => {
  vi.clearAllMocks()
  useWorkspaceListStore.setState({ data: idle() })
  useHomeWorkspaceStore.setState({ workspace: null })
  useSidebarStore.setState({ repos: [repo()] })
})

describe('repo name: click opens, double-click renames', () => {
  // The bug this file first pinned: the name is the biggest target on the row
  // and clicking it has to open the workspace, exactly like clicking anywhere
  // else on the row does.
  it('navigates on a single click of the repo name — with nothing to wait for', async () => {
    const user = userEvent.setup()
    render(<WorkspaceTree />)

    await user.click(screen.getByText('crowbar'))

    // Asserted with no timer advanced: a deferred navigation would still be
    // sitting in a pending callback here and this would read zero.
    expect(navigateFn).toHaveBeenCalledTimes(1)
    expect(screen.queryByDisplayValue('crowbar')).not.toBeInTheDocument()
  })

  it('opens the rename editor when the repo name is double-clicked', async () => {
    const user = userEvent.setup()
    render(<WorkspaceTree />)

    await user.dblClick(screen.getByText('crowbar'))

    expect(screen.getByDisplayValue('crowbar')).toBeInTheDocument()
  })

  // The accepted cost of dropping the timer, pinned so it reads as a decision
  // rather than a regression: the clicks that make up the double-click still
  // reach the row, so renaming lands you in the repo home first. Benign — that
  // is the home of the very repo you are renaming, and the sidebar carrying the
  // editor is not torn down by the navigation.
  it('also navigates while double-clicking, and keeps the editor open', async () => {
    const user = userEvent.setup()
    render(<WorkspaceTree />)

    await user.dblClick(screen.getByText('crowbar'))

    expect(navigateFn).toHaveBeenCalled()
    expect(screen.getByDisplayValue('crowbar')).toBeInTheDocument()
  })

  it('navigates on a single click elsewhere on the repo row', async () => {
    const user = userEvent.setup()
    render(<WorkspaceTree />)

    await user.click(screen.getByLabelText('Open crowbar'))

    expect(navigateFn).toHaveBeenCalledTimes(1)
  })

  // The branch row has always worked this way; the repo header now matches it
  // instead of being the one row in the tree with a delay.
  it('opens the rename editor when the branch name is double-clicked', async () => {
    const user = userEvent.setup()
    render(<WorkspaceTree />)

    await user.dblClick(screen.getByText('feature/x'))

    expect(screen.getByDisplayValue('feature/x')).toBeInTheDocument()
  })
})

/**
 * A folder answers the same two gestures, through the same editor.
 *
 * It has to: grouping rows makes a folder named for you, so the name is
 * something you say afterwards or never. A folder holds no branch and no
 * worktree, so committing one is a PATCH that moves nothing on disk — and, like
 * every other rename in this tree, the row is left alone until the stream says
 * the daemon took it.
 */
describe('folder name: click folds, double-click renames', () => {
  beforeEach(() => {
    useSidebarStore.setState({
      repos: [repo({ folders: [{ id: 'f1', repoId: 'r1', name: 'spikes', order: 0 }] })],
    })
  })

  it('opens the rename editor when the folder name is double-clicked', async () => {
    const user = userEvent.setup()
    render(<WorkspaceTree />)

    await user.dblClick(screen.getByText('spikes'))

    expect(screen.getByDisplayValue('spikes')).toBeInTheDocument()
  })

  // The same accepted cost the repo row pins above: both clicks reach the row,
  // so a folder folds and unfolds on its way to the editor and lands back where
  // it started. Nothing is held open waiting to find out.
  it('leaves the folder open as it was, with the editor up', async () => {
    const user = userEvent.setup()
    render(<WorkspaceTree />)

    await user.dblClick(screen.getByText('spikes'))

    expect(useSidebarStore.getState().collapsedWorkspaces.has('f1')).toBe(false)
    expect(screen.getByDisplayValue('spikes')).toBeInTheDocument()
  })

  it('sends the new name on Enter', async () => {
    const user = userEvent.setup()
    render(<WorkspaceTree />)

    await user.dblClick(screen.getByText('spikes'))
    // The editor opens with the name selected, so typing replaces it.
    await user.keyboard('experiments{Enter}')

    expect(placeFolder).toHaveBeenCalledExactlyOnceWith('p1', 'r1', 'f1', { name: 'experiments' })
    // Not relabelled here: the FolderDTO is what renames the row.
    expect(screen.getByText('spikes')).toBeInTheDocument()
  })

  it('sends nothing on Escape', async () => {
    const user = userEvent.setup()
    render(<WorkspaceTree />)

    await user.dblClick(screen.getByText('spikes'))
    await user.keyboard('experiments{Escape}')

    expect(placeFolder).not.toHaveBeenCalled()
    expect(screen.getByText('spikes')).toBeInTheDocument()
  })
})
