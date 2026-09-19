import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { renderHook } from '@testing-library/react'

vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/editor/stores/buffer-session-persistence', () => ({
  saveSessionToStore: vi.fn(),
  clearQueuedWorkspaceSessionSave: vi.fn(),
}))

import { useIdeShellWorkspaceRetention } from '@/components/layout/use-ide-shell-workspace-retention'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import { getInitialState, useSidebarStore, type Repo } from '@/lib/store/sidebar'

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
  useSidebarStore.setState(getInitialState())
  resetWindowPaneStoreForTests()
})

afterEach(() => {
  resetWindowPaneStoreForTests()
})

/**
 * The exact live-reported bug: opening a project-home-scoped chat shows the
 * empty "No folder open" file explorer until the user sends a message — at
 * which point it "automatically resolves the correct workspace". The active
 * pane's own chat resolution (`activePaneWorkspaceId`) is null until some
 * workspace store has been seeded with the chat, which — for a project-home
 * chat, which rides no repo (`Repo.chats` never carries it) — only happens
 * once something mounts the home workspace's own store. This hook must not
 * wait for that: the home workspace's id/path are already known the instant
 * `GET /home` answers (home-workspace-resolver.ts), independent of any chat.
 */
describe('useIdeShellWorkspaceRetention — project-home chat, before it resolves via a pane', () => {
  it("resolves sidebarWorkspacePath to the home workspace's REAL path, not the empty state", () => {
    useSidebarStore.getState().setRepos([repoWithOwnPath])
    const { activePaneId, paneActions } = windowPaneStore.getState()
    // A freshly opened project-home chat: no workspace store has it yet, and
    // it is not (and never is) present in any repo's own `chats` array, so
    // the sidebar hint this hook's `activePaneWorkspaceId` leans on cannot
    // name a workspace for it either.
    paneActions.setPaneChat(activePaneId, 'home-chat-1', null)

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
    // Not the empty state, and not some unrelated repo's own checkout —
    // borrowing repoWithOwnPath's directory as a stand-in for "the project's
    // root" is exactly the wrong-but-non-empty answer the old fallback gave.
    expect(result.current.sidebarWorkspacePath).not.toBe('')
    expect(result.current.sidebarWorkspacePath).not.toBe(repoWithOwnPath.localPath)
  })

  it("still falls back to a project repo's path when the real home path is not known yet", () => {
    useSidebarStore.getState().setRepos([repoWithOwnPath])
    const { activePaneId, paneActions } = windowPaneStore.getState()
    paneActions.setPaneChat(activePaneId, 'home-chat-1', null)

    const { result } = renderHook(() =>
      useIdeShellWorkspaceRetention(undefined, 'ws-home-1', 'p1', undefined, true, null),
    )

    // Graceful degradation, unchanged from before this fix: some real
    // directory under the project beats an empty file explorer.
    expect(result.current.sidebarWorkspacePath).toBe(repoWithOwnPath.localPath)
  })

  it('effectiveActiveWorkspaceId still resolves to the home workspace either way', () => {
    const { activePaneId, paneActions } = windowPaneStore.getState()
    paneActions.setPaneChat(activePaneId, 'home-chat-1', null)

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
