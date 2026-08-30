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
import { render, screen } from '@testing-library/react'

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

  // Spec §6: "the tree keeps a bottom inset the height of the card" —
  // threaded straight through to SpaceScroller, not re-derived here.
  it('threads bottomInset through to the scroll region as a bottom padding', () => {
    useSidebarStore.setState({
      repos: [repo({ id: 'r1', projectId: 'p1', defaultWorkspaceId: 'home-a' })],
    })

    render(
      <SidebarTreeSurface
        projects={[projectA]}
        activeProjectId="p1"
        onActiveProjectChange={vi.fn()}
        bottomInset={300}
      />,
    )

    expect(screen.getByTestId('space-scroll-content')).toHaveStyle({ paddingBottom: '300px' })
  })
})
