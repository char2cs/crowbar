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
import type { Repo } from '@/core/entities/repo-types'
import {
  EMPTY_FOCUSED_WORKSPACE_CONTEXT,
  useFocusedWorkspaceContextStore,
} from '@/features/window/stores/focused-workspace-context-store'
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

// A project-home chat (in no repo) has no pane-resolved workspace until a store is seeded, yet the
// file explorer must show the home workspace at once: its id and path are known when `GET /home`
// answers, independent of any chat.
describe('useIdeShellWorkspaceRetention — project-home chat, before it resolves via a pane', () => {
  it("resolves sidebarWorkspacePath to the home workspace's REAL path, not the empty state", () => {
    seedRepos([repoWithOwnPath])
    const { paneActions } = windowPaneStore.getState()
    // A freshly opened project-home chat: no workspace store has it yet, and
    // it is not (and never is) present in any repo's own `chats` array, so
    // the sidebar hint this hook's `activePaneWorkspaceId` leans on cannot
    // name a workspace for it either.
    paneActions.openChat('home-chat-1')

    const { result } = renderHook(() =>
      useIdeShellWorkspaceRetention(
        undefined, // activeWorkspaceId — the home route has no repoId/wsId segment
        'ws-home-1', // homeWorkspaceId — already resolved by ide-shell.tsx
        'p1',
        undefined,
        true, // isHomeRoute
        '/Users/mateo/projects/rabbyte-labs', // homeWorkspacePath — GET /home's own localPath
      ),
    )

    expect(result.current.sidebarWorkspacePath).toBe('/Users/mateo/projects/rabbyte-labs')
    // Neither empty nor another repo's checkout standing in for "the project's root".
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

    // Graceful degradation: some real directory under the project beats an empty file explorer.
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

// A view can split a home chat's pane with a branch chat's pane; focusing the branch pane flips
// the explorer to its worktree though the route stays on home. The pane store guarantees the
// focused pane belongs to the showing view, so its workspace is never stale and must win.
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
    // The split's OTHER pane: a branch-workspace chat, focused, while the
    // route is still on project home.
    paneActions.openChat('branch-chat-1', { workspaceId: 'ws-other-repo' })

    const { result } = renderHook(() =>
      useIdeShellWorkspaceRetention(
        undefined, // activeWorkspaceId — the home route has no repoId/wsId segment
        'ws-home-1', // homeWorkspaceId — already resolved by ide-shell.tsx
        'p1',
        undefined,
        true, // isHomeRoute
        '/Users/mateo/projects/rabbyte-labs',
      ),
    )

    expect(result.current.effectiveActiveWorkspaceId).toBe('ws-other-repo')
  })

  it('falls back to the home workspace when the active pane is the chatless stage (no branch pane focused)', () => {
    seedRepos([repoWithOwnPath])
    // No `openChat` call: the active pane is the chatless stage/tray, so
    // `activePaneWorkspaceId` resolves to null.

    const { result } = renderHook(() =>
      useIdeShellWorkspaceRetention(
        undefined,
        'ws-home-1',
        'p1',
        undefined,
        true, // isHomeRoute
        '/Users/mateo/projects/rabbyte-labs',
      ),
    )

    expect(result.current.effectiveActiveWorkspaceId).toBe('ws-home-1')
  })
})

// A home chat's pane is active while the ROUTE is a repo workspace (`isHomeRoute` false): a
// correctly resolved `homeWorkspaceId` is honoured regardless of route. The shell's half of the
// wiring is ide-shell.test.tsx's "mixed split".
describe('useIdeShellWorkspaceRetention — project-home pane, route on a DIFFERENT (repo) workspace of the same project', () => {
  afterEach(() => {
    destroyWorkspaceStore('ws-home-1')
  })

  it("resolves sidebarWorkspacePath to the home workspace's own path even though isHomeRoute is false", () => {
    seedRepos([repoWithOwnPath])
    const { paneActions } = windowPaneStore.getState()
    // The pane's chat resolves to home via the REGISTRY (the sidebar hint can't name a home chat),
    // as in the real app once the chat has streamed.
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
        'ws-a', // activeWorkspaceId — the route's own repo workspace
        'ws-home-1', // homeWorkspaceId — resolved for the active PROJECT, not the route
        'p1',
        'r1',
        false, // isHomeRoute — the route is on a repo, not home
        '/Users/mateo/projects/rabbyte-labs',
      ),
    )

    expect(result.current.sidebarWorkspacePath).toBe('/Users/mateo/projects/rabbyte-labs')
    expect(result.current.sidebarWorkspacePath).not.toBe('')
    expect(result.current.sidebarWorkspacePath).not.toBe(repoWithOwnPath.localPath)
  })
})

// Branch-out and move re-point a chat to another workspace; its pane keeps the workspace it
// opened in. The Files tab, the git tab and store retention must follow the chat.
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
    act(() => useFocusedWorkspaceContextStore.setState({ ...EMPTY_FOCUSED_WORKSPACE_CONTEXT }))
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
