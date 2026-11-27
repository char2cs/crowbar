import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'

vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))

import { useIdeShellWorkspaceRetention } from '@/components/layout/use-ide-shell-workspace-retention'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { resetWindowPaneStoreForTests } from '@/__tests__/__fixtures__/window-pane-store'
import type { Repo } from '@/core/entities/repo-types'
import { useFocusedWorkspaceContextStore } from '@/features/window/stores/focused-workspace-context-store'
import { destroyWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { nextVersion, writeChat } from '@/__tests__/__fixtures__/agent-chat'
import { resetEntities, seedRepos } from '@/__tests__/__fixtures__/sidebar-entities'

const repoWithOwnPath: Repo = {
  id: 'r1',
  projectId: 'p1',
  name: 'crowbar',
  avatarLabel: 'C',
  avatarColor: 'c',
  localPath: '/Users/mateo/projects/rabbyte-labs/crowbar',
  workspaces: [],
}

beforeEach(() => {
  resetEntities()
  resetWindowPaneStoreForTests()
})

afterEach(() => {
  resetWindowPaneStoreForTests()
})

describe('useIdeShellWorkspaceRetention — project-home chat, before it resolves via a pane', () => {
  it("resolves sidebarWorkspacePath to the home workspace's REAL path, not the empty state", () => {
    seedRepos([repoWithOwnPath])
    const { paneActions } = windowPaneStore.getState()
    paneActions.openChat('home-chat-1')

    const { result } = renderHook(() =>
      useIdeShellWorkspaceRetention(
        undefined,
        'ws-home-1',
        'p1',
        undefined,
        true,
        '/Users/mateo/projects/rabbyte-labs',
      ),
    )

    expect(result.current.sidebarWorkspacePath).toBe('/Users/mateo/projects/rabbyte-labs')
    expect(result.current.sidebarWorkspacePath).not.toBe('')
    expect(result.current.sidebarWorkspacePath).not.toBe(repoWithOwnPath.localPath)
  })

  it("still falls back to a project repo's path when the real home path is not known yet", () => {
    seedRepos([repoWithOwnPath])
    const { paneActions } = windowPaneStore.getState()
    paneActions.openChat('home-chat-1')

    const { result } = renderHook(() =>
      useIdeShellWorkspaceRetention(undefined, 'ws-home-1', 'p1', undefined, true, null),
    )

    expect(result.current.sidebarWorkspacePath).toBe(repoWithOwnPath.localPath)
  })

  it('effectiveActiveWorkspaceId still resolves to the home workspace either way', () => {
    const { paneActions } = windowPaneStore.getState()
    paneActions.openChat('home-chat-1')

    const { result } = renderHook(() =>
      useIdeShellWorkspaceRetention(
        undefined,
        'ws-home-1',
        'p1',
        undefined,
        true,
        '/Users/mateo/projects/rabbyte-labs',
      ),
    )

    expect(result.current.effectiveActiveWorkspaceId).toBe('ws-home-1')
  })
})

describe('useIdeShellWorkspaceRetention — home route with a split pane focused on a branch workspace', () => {
  it('resolves to the focused pane workspace, not home, when the active pane holds a branch-workspace chat', () => {
    seedRepos([
      {
        id: 'r2',
        projectId: 'p1',
        name: 'other-repo',
        avatarLabel: 'O',
        avatarColor: 'o',
        localPath: '/Users/mateo/projects/other-repo',
        workspaces: [],
        chats: [
          {
            id: 'branch-chat-1',
            repoId: 'r2',
            title: 'branch',
            order: 0,
            workspaceId: 'ws-other-repo',
          },
        ],
      },
    ])
    const { paneActions } = windowPaneStore.getState()
    paneActions.openChat('branch-chat-1', { workspaceId: 'ws-other-repo' })

    const { result } = renderHook(() =>
      useIdeShellWorkspaceRetention(
        undefined,
        'ws-home-1',
        'p1',
        undefined,
        true,
        '/Users/mateo/projects/rabbyte-labs',
      ),
    )

    expect(result.current.effectiveActiveWorkspaceId).toBe('ws-other-repo')
  })

  it('falls back to the home workspace when the active pane is the chatless stage (no branch pane focused)', () => {
    seedRepos([repoWithOwnPath])

    const { result } = renderHook(() =>
      useIdeShellWorkspaceRetention(
        undefined,
        'ws-home-1',
        'p1',
        undefined,
        true,
        '/Users/mateo/projects/rabbyte-labs',
      ),
    )

    expect(result.current.effectiveActiveWorkspaceId).toBe('ws-home-1')
  })
})

describe('useIdeShellWorkspaceRetention — project-home pane, route on a DIFFERENT (repo) workspace of the same project', () => {
  afterEach(() => {
    destroyWorkspaceStore('ws-home-1')
  })

  it("resolves sidebarWorkspacePath to the home workspace's own path even though isHomeRoute is false", () => {
    seedRepos([repoWithOwnPath])
    const { paneActions } = windowPaneStore.getState()
    writeChat({
      id: 'home-chat-1',
      workspaceId: 'ws-home-1',
      title: 'home-chat-1',
      liveRunnerId: '',
      terminalSessionId: '',
      activeProviderId: 'claude',
      working: false,
      version: nextVersion(),
      phase: 'dormant',
      createdAt: '2026-01-01T00:00:00Z',
      order: 0,
    })
    paneActions.openChat('home-chat-1', { workspaceId: 'ws-home-1' })

    const { result } = renderHook(() =>
      useIdeShellWorkspaceRetention(
        'ws-a',
        'ws-home-1',
        'p1',
        'r1',
        false,
        '/Users/mateo/projects/rabbyte-labs',
      ),
    )

    expect(result.current.sidebarWorkspacePath).toBe('/Users/mateo/projects/rabbyte-labs')
    expect(result.current.sidebarWorkspacePath).not.toBe('')
    expect(result.current.sidebarWorkspacePath).not.toBe(repoWithOwnPath.localPath)
  })
})

describe('useIdeShellWorkspaceRetention — a chat that moved to a child worktree', () => {
  const todoCli: Repo = {
    id: 'r-todo',
    projectId: 'p1',
    name: 'todo-cli',
    avatarLabel: 'T',
    avatarColor: 't',
    localPath: '/repos/todo-cli',
    defaultWorkspaceId: 'ws-main',
    workspaces: [
      { id: 'ws-main', branch: 'main', age: '', localPath: '/repos/todo-cli' },
      { id: 'ws-child', branch: 'wobbly', age: '', localPath: '/wt/wobbly', parentId: 'ws-main' },
    ],
  }

  afterEach(() => {
    act(() =>
      useFocusedWorkspaceContextStore.setState({
        ...useFocusedWorkspaceContextStore.getInitialState(),
      }),
    )
  })

  it("scopes the panel and retention to the chat's current workspace, not the pane's record", () => {
    seedRepos([todoCli])
    writeChat({ id: 'chat-b', repoId: 'r-todo', workspaceId: 'ws-main' })
    windowPaneStore.getState().paneActions.openChat('chat-b', { workspaceId: 'ws-main' })
    const { result } = renderHook(() =>
      useIdeShellWorkspaceRetention('ws-main', 'ws-home', 'p1', 'r-todo', false),
    )

    act(() => writeChat({ id: 'chat-b', workspaceId: 'ws-child' }))

    expect(result.current.effectiveActiveWorkspaceId).toBe('ws-child')
    expect(result.current.sidebarWorkspacePath).toBe('/wt/wobbly')
    expect(useFocusedWorkspaceContextStore.getState()).toMatchObject({
      workspaceId: 'ws-child',
      repoId: 'r-todo',
      rootPath: '/wt/wobbly',
    })
    expect(result.current.viewWsIds).toEqual(['ws-child'])
  })
})
