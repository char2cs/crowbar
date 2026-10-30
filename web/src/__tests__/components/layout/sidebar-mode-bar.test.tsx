import { act, fireEvent, render, screen, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/panes/lib/release-closed-chat', () => ({
  releaseClosedChat: vi.fn(async () => {}),
}))

import { SidebarModeBar } from '@/components/layout/sidebar-mode-bar'
import { useIdeShellWorkspaceRetention } from '@/components/layout/use-ide-shell-workspace-retention'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import { getInitialState, useSidebarStore, type Repo } from '@/lib/store/sidebar'
import {
  EMPTY_FOCUSED_WORKSPACE_CONTEXT,
  publishFocusedWorkspaceContext,
  type FocusedWorkspaceContext,
} from '@/features/window/stores/focused-workspace-context-store'

const REPO_CTX: FocusedWorkspaceContext = {
  projectId: 'p1',
  workspaceId: 'ws-repo',
  isProjectHome: false,
  repoId: 'r1',
  repoPath: '/repo',
  rootPath: '/repo',
}
const HOME_CTX: FocusedWorkspaceContext = {
  projectId: 'p1',
  workspaceId: 'ws-home',
  isProjectHome: true,
  repoId: null,
  repoPath: null,
  rootPath: '/projects/p1',
}

function segment(name: string): HTMLElement {
  return screen.getByRole('tab', { name })
}

describe('SidebarModeBar', () => {
  beforeEach(() => {
    useSidebarStore.setState(getInitialState())
    publishFocusedWorkspaceContext(REPO_CTX)
  })

  it('holds Chats, Files and Changes, in that order', () => {
    render(<SidebarModeBar />)
    expect(screen.getAllByRole('tab').map((t) => t.getAttribute('aria-label'))).toEqual([
      'Chats',
      'Files',
      'Changes',
    ])
  })

  it('renders the segments inside one labelled tablist', () => {
    render(<SidebarModeBar />)
    const list = screen.getByRole('tablist', { name: 'Sidebar mode' })
    expect(within(list).getAllByRole('tab')).toHaveLength(3)
  })

  it('selects Chats by default', () => {
    render(<SidebarModeBar />)
    expect(segment('Chats')).toHaveAttribute('aria-selected', 'true')
    expect(segment('Files')).toHaveAttribute('aria-selected', 'false')
    expect(segment('Changes')).toHaveAttribute('aria-selected', 'false')
  })

  it('selecting a segment writes the mode to the store', () => {
    render(<SidebarModeBar />)
    fireEvent.click(segment('Changes'))
    expect(useSidebarStore.getState().activeTab).toBe('git')
    expect(segment('Changes')).toHaveAttribute('aria-selected', 'true')
    expect(segment('Chats')).toHaveAttribute('aria-selected', 'false')
  })

  describe('availability follows the focused workspace', () => {
    it('Git is disabled for a workspace with no repo, and cannot be selected', () => {
      publishFocusedWorkspaceContext(HOME_CTX)
      render(<SidebarModeBar />)
      expect(screen.queryByRole('tab', { name: 'Changes' })).not.toBeInTheDocument()
      expect(screen.getAllByRole('tab')).toHaveLength(2)
      expect(segment('Files')).toHaveAttribute('aria-disabled', 'false')
    })

    it('with no focused workspace, Files and Git are disabled and Chats stays usable', () => {
      publishFocusedWorkspaceContext(EMPTY_FOCUSED_WORKSPACE_CONTEXT)
      render(<SidebarModeBar />)
      expect(segment('Chats')).toHaveAttribute('aria-disabled', 'false')
      expect(segment('Files')).toHaveAttribute('aria-disabled', 'true')
      expect(screen.queryByRole('tab', { name: 'Changes' })).not.toBeInTheDocument()
    })

    it('shows Chats as selected once the chosen mode becomes unavailable', () => {
      useSidebarStore.setState({ activeTab: 'git' })
      render(<SidebarModeBar />)
      expect(segment('Changes')).toHaveAttribute('aria-selected', 'true')

      act(() => publishFocusedWorkspaceContext(HOME_CTX))

      expect(screen.queryByRole('tab', { name: 'Changes' })).not.toBeInTheDocument()
      expect(segment('Chats')).toHaveAttribute('aria-selected', 'true')
    })
  })
})

const REPO: Repo = {
  id: 'r1',
  projectId: 'p1',
  name: 'crowbar',
  avatarLabel: 'C',
  avatarColor: 'c',
  localPath: '/repo',
  workspaces: [],
  defaultWorkspaceId: 'ws-repo',
}

interface Route {
  workspaceId: string | undefined
  repoId: string | undefined
  isHomeRoute: boolean
}
const HOME_ROUTE: Route = { workspaceId: undefined, repoId: undefined, isHomeRoute: true }
const REPO_ROUTE: Route = { workspaceId: 'ws-repo', repoId: 'r1', isHomeRoute: false }

/** The shell's real publisher feeding the bar, as ide-shell wires them. */
function ShellSidebar({ route }: { route: Route }) {
  useIdeShellWorkspaceRetention(
    route.workspaceId,
    'ws-home',
    'p1',
    route.repoId,
    route.isHomeRoute,
    '/projects/p1',
    '/projects/p1',
  )
  return <SidebarModeBar />
}

describe('SidebarModeBar on the empty stage', () => {
  beforeEach(() => {
    resetWindowPaneStoreForTests()
    useSidebarStore.setState(getInitialState())
    useSidebarStore.getState().setRepos([REPO])
    useSidebarStore.setState({ activeTab: 'files' })
  })

  it.each([
    ['project home', HOME_ROUTE],
    ['a repo workspace', REPO_ROUTE],
  ])('on %s with nothing open, Files and Git are disabled and Chats is shown', (_, route) => {
    render(<ShellSidebar route={route} />)
    expect(screen.getByRole('tablist', { name: 'Sidebar mode' })).toBeInTheDocument()
    expect(segment('Chats')).toHaveAttribute('aria-disabled', 'false')
    expect(segment('Files')).toHaveAttribute('aria-disabled', 'true')
    expect(screen.queryByRole('tab', { name: 'Changes' })).not.toBeInTheDocument()
    expect(segment('Chats')).toHaveAttribute('aria-selected', 'true')
  })

  it('closing the last view returns to the empty stage and drops Files', () => {
    const { paneActions } = windowPaneStore.getState()
    act(() => paneActions.openChat('chat-1', { workspaceId: 'ws-repo', projectId: 'p1' }))
    render(<ShellSidebar route={REPO_ROUTE} />)
    expect(segment('Files')).toHaveAttribute('aria-disabled', 'false')
    expect(segment('Changes')).toHaveAttribute('aria-disabled', 'false')
    expect(segment('Files')).toHaveAttribute('aria-selected', 'true')

    act(() => paneActions.closePane(windowPaneStore.getState().activePaneId))

    expect(segment('Files')).toHaveAttribute('aria-disabled', 'true')
    expect(screen.queryByRole('tab', { name: 'Changes' })).not.toBeInTheDocument()
    expect(segment('Chats')).toHaveAttribute('aria-selected', 'true')
    expect(useSidebarStore.getState().activeTab).toBe('files')
  })
})
