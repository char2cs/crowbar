/**
 * Fix-round regression coverage for two review findings on SidebarTreePanel:
 *
 *   - "New Project" (the app's only entry point for a second project once
 *     past the zero-project /oobe screen) was dropped when workspace-tree.tsx
 *     was deleted. Pinned here so it can never silently regress again.
 *   - Creating a workspace off the repo-home row used to post with NO
 *     parentId at all, which the backend reads as "no fork parent" —
 *     MergeEligibility keys on ws.ParentID != "", so that workspace could
 *     never merge back. Pinned against the real placement the row's own id
 *     produces.
 */
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

const navigate = vi.fn()
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => navigate,
  // RemovalTray (mounted inside SidebarTreePanel) needs both.
  useRouter: () => ({ state: { location: { pathname: '/' } } }),
  useRouterState: () => '/',
}))

const { postWorkspace, importProjectAndSync, createChat, toastError } = vi.hoisted(() => ({
  postWorkspace: vi.fn(() => Promise.resolve()),
  importProjectAndSync: vi.fn(),
  createChat: vi.fn(() => Promise.resolve('chat-1')),
  toastError: vi.fn(),
}))

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  postWorkspace,
}))
vi.mock('@/lib/store/projects', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/store/projects')>()),
  importProjectAndSync,
}))
vi.mock('@/features/agent/api/agent-api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/agent/api/agent-api')>()),
  createChat,
}))
vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: toastError, success: vi.fn(), info: vi.fn() },
}))
// The real modal drives a Tauri native-dialog / postProject flow this file
// has no business exercising — SidebarTreePanel's own contract is just "open
// it, and hand its result to importProjectAndSync", which this stub proves
// without any of that.
vi.mock('@/components/projects/import-project-modal', () => ({
  ImportProjectModal: ({
    open,
    onImport,
  }: {
    open: boolean
    onImport: (p: { id: string; name: string; path: string; lastActivity: Date }) => void
  }) =>
    open ? (
      <button
        type="button"
        onClick={() =>
          onImport({ id: 'p2', name: 'second-project', path: '/p2', lastActivity: new Date(0) })
        }
      >
        confirm-import
      </button>
    ) : null,
}))

import { SidebarTreePanel } from '@/components/layout/sidebar-tree-panel'
import { getInitialState, useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { getInitialRemovalState, useRemovalTrayStore } from '@/lib/store/sidebar-removal'

const repo = (over: Partial<Repo> = {}): Repo => ({
  id: 'r1',
  projectId: 'p1',
  name: 'crowbar',
  avatarLabel: 'C',
  avatarColor: 'bg-indigo-700',
  defaultWorkspaceId: 'home-1',
  defaultBranch: 'main',
  workspaces: [],
  folders: [],
  ...over,
})

beforeEach(() => {
  vi.clearAllMocks()
  useSidebarStore.setState(getInitialState())
  useRemovalTrayStore.setState(getInitialRemovalState())
})

describe('New Project entry point', () => {
  it('renders a "New Project" row', () => {
    useSidebarStore.setState({ repos: [repo()] })
    render(<SidebarTreePanel />)
    expect(screen.getByText('New Project')).toBeInTheDocument()
  })

  it('opens the import modal on click, and importing closes it and syncs the project', async () => {
    const user = userEvent.setup()
    useSidebarStore.setState({ repos: [repo()] })
    render(<SidebarTreePanel />)

    expect(screen.queryByText('confirm-import')).not.toBeInTheDocument()
    await user.click(screen.getByText('New Project'))
    expect(screen.getByText('confirm-import')).toBeInTheDocument()

    await user.click(screen.getByText('confirm-import'))
    expect(importProjectAndSync).toHaveBeenCalledExactlyOnceWith(
      expect.objectContaining({ id: 'p2', name: 'second-project' }),
    )
    expect(screen.queryByText('confirm-import')).not.toBeInTheDocument()
  })
})

describe('creating a workspace off the repo-home row', () => {
  it('posts with the repo-home id as parentId, never an empty placement', async () => {
    const user = userEvent.setup()
    useSidebarStore.setState({ repos: [repo()] })
    render(<SidebarTreePanel />)

    await user.click(screen.getByRole('button', { name: /new workspace under crowbar/i }))

    expect(postWorkspace).toHaveBeenCalledExactlyOnceWith(
      'p1',
      'r1',
      expect.stringMatching(/^workspace-/),
      { parentId: 'home-1' },
    )
  })

  it('posts a real fork parent for a non-home workspace row too', async () => {
    const user = userEvent.setup()
    useSidebarStore.setState({
      repos: [repo({ workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }] })],
    })
    render(<SidebarTreePanel />)

    await user.click(screen.getByRole('button', { name: /new workspace under alpha/i }))

    expect(postWorkspace).toHaveBeenCalledExactlyOnceWith(
      'p1',
      'r1',
      expect.stringMatching(/^workspace-/),
      { parentId: 'ws-a' },
    )
  })
})

describe('starting a thread on an empty folder', () => {
  it('says why instead of silently doing nothing', async () => {
    const user = userEvent.setup()
    useSidebarStore.setState({
      repos: [repo({ folders: [{ id: 'f1', repoId: 'r1', name: 'spikes', order: 0 }] })],
    })
    render(<SidebarTreePanel />)

    await user.click(screen.getByTestId('affordance-dropdown'))
    await user.click(await screen.findByText('Create thread'))

    expect(toastError).toHaveBeenCalledOnce()
    expect(createChat).not.toHaveBeenCalled()
  })
})
