/**
 * `SidebarTreeSurface` wires `SpaceScroller` + the hoisted chrome
 * (RemovalTray/dialogs/context menu/"New Project") against the real
 * removal-tray-filtered `repos` — the thing `sidebar-tree-panel.tsx` used to
 * do for one flat tree, now scoped per project. Individual pieces (derivers,
 * handlers, RecentsBand wiring) have their own focused unit tests; this one
 * checks the WIRING: real multi-project repos genuinely produce one tree per
 * project, each excluding the other's rows.
 */
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'

const navigate = vi.fn()
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => navigate,
  useRouter: () => ({ state: { location: { pathname: '/' } } }),
  useRouterState: () => '/',
}))
// The real modal drives a Tauri native-dialog / postProject flow this file
// has no business exercising.
vi.mock('@/components/projects/import-project-modal', () => ({
  ImportProjectModal: () => null,
}))
// SpaceScroller's/RecentsBand's cross-store reads — no pane/chat fixtures are
// exercised here, so the registry can safely report nothing active.
vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getAllActiveWorkspaceIds: () => [],
  getOrCreateWorkspaceStore: () => ({
    getState: () => ({
      panes: {},
      agentChats: { working: {}, chats: [] },
      dormantArrangements: [],
    }),
    subscribe: () => () => {},
  }),
}))
// DeleteConfirmDialog's real preview fetch would otherwise hit the real
// (unmocked) apiFetch retry loop against a relative URL jsdom can't resolve —
// several real seconds of backoff before it finally rejects. Stubbed here so
// the trash-click wiring tests below resolve instantly.
const { fetchDeletePreview } = vi.hoisted(() => ({
  fetchDeletePreview: vi.fn().mockResolvedValue({ chatCount: 0, fileCount: 0 }),
}))
vi.mock('@/components/sidebar/lib/delete-preview-client', () => ({ fetchDeletePreview }))
const { toastError } = vi.hoisted(() => ({ toastError: vi.fn() }))
vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: toastError, success: vi.fn(), info: vi.fn() },
}))
const { deleteChat } = vi.hoisted(() => ({ deleteChat: vi.fn(() => Promise.resolve()) }))
vi.mock('@/features/agent/api/agent-api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/agent/api/agent-api')>()),
  deleteChat,
}))

import { SidebarTreeSurface } from '@/components/layout/sidebar-tree-surface'
import { getInitialState, useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { getInitialRemovalState, useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import type { Project } from '@/lib/types'

const projectA: Project = { id: 'p1', name: 'proj-a', path: '/p1', lastActivity: new Date(0) }
const projectB: Project = { id: 'p2', name: 'proj-b', path: '/p2', lastActivity: new Date(0) }

const repo = (over: Partial<Repo> = {}): Repo => ({
  id: 'r1',
  projectId: 'p1',
  name: 'crowbar',
  avatarLabel: 'C',
  avatarColor: 'bg-indigo-700',
  defaultWorkspaceId: 'home-1',
  workspaces: [],
  ...over,
})

beforeEach(() => {
  vi.clearAllMocks()
  fetchDeletePreview.mockResolvedValue({ chatCount: 0, fileCount: 0 })
  // jsdom does not implement scrollTo — SpaceScroller calls it on mount to
  // align its panel.
  HTMLElement.prototype.scrollTo = vi.fn()
  useSidebarStore.setState(getInitialState())
  useRemovalTrayStore.setState(getInitialRemovalState())
})

describe('SidebarTreeSurface', () => {
  it("shows each project's own repo, excluding the other project's", () => {
    useSidebarStore.setState({
      repos: [
        repo({ id: 'r1', projectId: 'p1', defaultWorkspaceId: 'home-a', name: 'repo-a' }),
        repo({ id: 'r2', projectId: 'p2', defaultWorkspaceId: 'home-b', name: 'repo-b' }),
      ],
    })

    render(
      <SidebarTreeSurface
        projects={[projectA, projectB]}
        activeProjectId="p1"
        onActiveProjectChange={vi.fn()}
      />,
    )

    const panels = screen.getAllByTestId('space-panel')
    expect(panels).toHaveLength(2)
    expect(panels[0]).toHaveTextContent('repo-a')
    expect(panels[0]).not.toHaveTextContent('repo-b')
    expect(panels[1]).toHaveTextContent('repo-b')
    expect(panels[1]).not.toHaveTextContent('repo-a')
  })

  it('mounts RemovalTray and "New Project" once, not once per project', () => {
    useSidebarStore.setState({
      repos: [
        repo({ id: 'r1', projectId: 'p1' }),
        repo({ id: 'r2', projectId: 'p2', defaultWorkspaceId: 'home-2' }),
      ],
    })

    render(
      <SidebarTreeSurface
        projects={[projectA, projectB]}
        activeProjectId="p1"
        onActiveProjectChange={vi.fn()}
      />,
    )

    expect(screen.getAllByText('New Project')).toHaveLength(1)
  })

  it('a row held in the removal tray disappears from its own project panel', () => {
    useSidebarStore.setState({
      repos: [
        repo({
          id: 'r1',
          projectId: 'p1',
          defaultWorkspaceId: 'home-a',
          workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }],
        }),
      ],
    })
    useRemovalTrayStore.setState({ hiddenIds: new Set(['ws-a']) })

    render(
      <SidebarTreeSurface
        projects={[projectA]}
        activeProjectId="p1"
        onActiveProjectChange={vi.fn()}
      />,
    )

    expect(screen.queryByText('alpha')).not.toBeInTheDocument()
  })

  // Task 25's own real-flow insertion point: a trash click must NOT hold the
  // row immediately any more — it has to open the confirm dialog first and
  // wait on a real Delete click.
  it('a trash click opens the confirm dialog instead of holding the row immediately', async () => {
    useSidebarStore.setState({
      repos: [
        repo({
          id: 'r1',
          projectId: 'p1',
          defaultWorkspaceId: 'home-a',
          workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }],
        }),
      ],
    })

    render(
      <SidebarTreeSurface
        projects={[projectA]}
        activeProjectId="p1"
        onActiveProjectChange={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Delete alpha' }))

    expect(useRemovalTrayStore.getState().entries).toEqual([])
    await waitFor(() => expect(screen.getByRole('dialog')).toBeInTheDocument())

    fireEvent.click(screen.getByRole('button', { name: /delete/i }))
    expect(useRemovalTrayStore.getState().entries).toHaveLength(1)
    expect(useRemovalTrayStore.getState().entries[0]?.id).toBe('ws-a')
  })

  // Review round 1, Important: a locked (non-home) workspace's trash button
  // still shows, but `handleTrash` finds zero drafts for it — confirming
  // must say so rather than silently doing nothing after walking the user
  // through a real confirm dialog.
  it("confirming a locked workspace's delete reports it instead of silently no-opping", async () => {
    useSidebarStore.setState({
      repos: [
        repo({
          id: 'r1',
          projectId: 'p1',
          defaultWorkspaceId: 'home-a',
          workspaces: [
            { id: 'ws-locked', branch: 'locked-one', age: '', order: 0, status: 'locked' },
          ],
        }),
      ],
    })

    render(
      <SidebarTreeSurface
        projects={[projectA]}
        activeProjectId="p1"
        onActiveProjectChange={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Delete locked-one' }))
    await waitFor(() => expect(screen.getByRole('dialog')).toBeInTheDocument())

    fireEvent.click(screen.getByRole('button', { name: /delete/i }))

    expect(useRemovalTrayStore.getState().entries).toEqual([])
    expect(toastError).toHaveBeenCalledExactlyOnceWith(
      expect.stringContaining('locked-one'),
    )
  })

  // `resolveRow` (space-content-actions.ts) cannot see a chat row, so
  // `deletingRepo`'s lookup has to try `resolveChatRow` first — otherwise the
  // dialog's `projectId` comes back undefined and the real preview fetch is
  // skipped for a chat specifically, degrading to the generic fallback copy.
  it("a chat row's trash resolves its owning repo, so the real delete-preview fires", async () => {
    useSidebarStore.setState({
      repos: [
        repo({
          id: 'r1',
          projectId: 'p1',
          defaultWorkspaceId: 'home-a',
          chats: [{ id: 'c1', repoId: 'r1', title: 'a chat', order: 0 }],
        }),
      ],
    })

    render(
      <SidebarTreeSurface
        projects={[projectA]}
        activeProjectId="p1"
        onActiveProjectChange={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Delete a chat' }))
    await waitFor(() => expect(screen.getByRole('dialog')).toBeInTheDocument())

    expect(fetchDeletePreview).toHaveBeenCalledExactlyOnceWith('p1', 'r1', 'c1')

    fireEvent.click(screen.getByRole('button', { name: /delete/i }))
    await waitFor(() => expect(deleteChat).toHaveBeenCalledExactlyOnceWith('home-a', 'c1'))
  })
})
