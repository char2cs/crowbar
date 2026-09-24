import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'

vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))

import { useIdeShellWorkspaceRetention } from '@/components/layout/use-ide-shell-workspace-retention'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import { getInitialState, useSidebarStore, type Repo } from '@/lib/store/sidebar'
import {
  getOrCreateWorkspaceStore,
  destroyWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import {
  EMPTY_FOCUSED_WORKSPACE_CONTEXT,
  getFocusedWorkspaceContext,
  publishFocusedWorkspaceContext,
} from '@/features/window/stores/focused-workspace-context-store'

const HOME_PATH = '/projects/p1'

const repo: Repo = {
  id: 'r1',
  projectId: 'p1',
  name: 'crowbar',
  avatarLabel: 'C',
  avatarColor: 'c',
  localPath: '/repos/crowbar',
  defaultWorkspaceId: 'ws-default',
  workspaces: [{ id: 'ws-branch', localPath: '/worktrees/feat-x' } as Repo['workspaces'][number]],
  chats: [{ id: 'branch-chat', repoId: 'r1', title: 'b', order: 0, workspaceId: 'ws-branch' }],
}

function seedHomeChat() {
  getOrCreateWorkspaceStore('ws-home').getState().upsertAgentChat({
    id: 'home-chat',
    workspaceId: 'ws-home',
    title: 'home',
    liveRunnerId: '',
    terminalSessionId: '',
    activeProviderId: 'claude',
    createdAt: '2026-01-01T00:00:00Z',
    order: 0,
  })
}

/** A view split into a home-chat pane and a branch-chat pane; returns both pane ids. */
function openSplit() {
  const { paneActions } = windowPaneStore.getState()
  paneActions.openChat('home-chat', { workspaceId: 'ws-home' })
  const homePane = windowPaneStore.getState().activePaneId
  const branchPane = paneActions.splitPane(homePane, 'horizontal')
  if (!branchPane) throw new Error('split failed')
  paneActions.setActivePane(branchPane)
  paneActions.openChat('branch-chat', { workspaceId: 'ws-branch' })
  return { homePane, branchPane }
}

beforeEach(() => {
  useSidebarStore.setState(getInitialState())
  useSidebarStore.getState().setRepos([repo])
  resetWindowPaneStoreForTests()
  publishFocusedWorkspaceContext(EMPTY_FOCUSED_WORKSPACE_CONTEXT)
  seedHomeChat()
})

afterEach(() => {
  resetWindowPaneStoreForTests()
  destroyWorkspaceStore('ws-home')
})

describe('focused workspace context — published where the active workspace is resolved', () => {
  it('home route, branch pane focused: context is the branch repo workspace', () => {
    openSplit()
    renderHook(() =>
      useIdeShellWorkspaceRetention(
        undefined,
        'ws-home',
        'p1',
        undefined,
        true,
        HOME_PATH,
        HOME_PATH,
      ),
    )
    expect(getFocusedWorkspaceContext()).toEqual({
      projectId: 'p1',
      workspaceId: 'ws-branch',
      isProjectHome: false,
      repoId: 'r1',
      repoPath: '/worktrees/feat-x',
      rootPath: '/worktrees/feat-x',
    })
  })

  it('focus moves to the home pane: context becomes project home with no repo', () => {
    const { homePane } = openSplit()
    renderHook(() =>
      useIdeShellWorkspaceRetention(
        undefined,
        'ws-home',
        'p1',
        undefined,
        true,
        HOME_PATH,
        HOME_PATH,
      ),
    )
    act(() => windowPaneStore.getState().paneActions.setActivePane(homePane))
    expect(getFocusedWorkspaceContext()).toEqual({
      projectId: 'p1',
      workspaceId: 'ws-home',
      isProjectHome: true,
      repoId: null,
      repoPath: null,
      rootPath: HOME_PATH,
    })
  })

  it('no split, repo route: context is the routed workspace (repo known from the route alone)', () => {
    useSidebarStore.getState().setRepos([])
    renderHook(() =>
      useIdeShellWorkspaceRetention(
        'ws-routed',
        'ws-home',
        'p1',
        'r9',
        false,
        HOME_PATH,
        HOME_PATH,
      ),
    )
    expect(getFocusedWorkspaceContext()).toMatchObject({
      workspaceId: 'ws-routed',
      isProjectHome: false,
      repoId: 'r9',
    })
  })

  it('home route, nothing focused and home unresolved: still project home, no repo', () => {
    renderHook(() =>
      useIdeShellWorkspaceRetention(undefined, null, 'p1', undefined, true, null, HOME_PATH),
    )
    expect(getFocusedWorkspaceContext()).toMatchObject({
      workspaceId: null,
      isProjectHome: true,
      repoId: null,
    })
  })
})
