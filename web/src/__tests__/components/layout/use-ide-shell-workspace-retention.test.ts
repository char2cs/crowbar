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
import {
  getOrCreateWorkspaceStore,
  destroyWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'

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
    // Not the empty state, and not some unrelated repo's own checkout —
    // borrowing repoWithOwnPath's directory as a stand-in for "the project's
    // root" is exactly the wrong-but-non-empty answer the old fallback gave.
    expect(result.current.sidebarWorkspacePath).not.toBe('')
    expect(result.current.sidebarWorkspacePath).not.toBe(repoWithOwnPath.localPath)
  })

  it("still falls back to a project repo's path when the real home path is not known yet", () => {
    useSidebarStore.getState().setRepos([repoWithOwnPath])
    const { paneActions } = windowPaneStore.getState()
    paneActions.openChat('home-chat-1')

    const { result } = renderHook(() =>
      useIdeShellWorkspaceRetention(undefined, 'ws-home-1', 'p1', undefined, true, null),
    )

    // Graceful degradation, unchanged from before this fix: some real
    // directory under the project beats an empty file explorer.
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

/**
 * The other half of the same live bug, in its CURRENT form (views-as-tabs):
 * a view can split a project-home chat's pane alongside a branch-workspace
 * chat's pane. Focusing the branch pane must flip the file explorer to the
 * branch worktree even though the route stays on `/ide/<project>/home` — a
 * pane never navigates. The old fallback here ("home always wins on the
 * home route") was written for a DIFFERENT, now-impossible case — a stale
 * pane left over from a previously-visited repo, back when nothing cleared
 * `windowPaneStore`'s pane/chatId state on navigating to home. The pane
 * store's integrity invariant now guarantees `activePaneId` is a pane of the
 * showing view, so a focused pane's workspace is never stale; it must win.
 */
describe('useIdeShellWorkspaceRetention — home route with a split pane focused on a branch workspace', () => {
  it('resolves to the focused pane workspace, not home, when the active pane holds a branch-workspace chat', () => {
    useSidebarStore.getState().setRepos([
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
    useSidebarStore.getState().setRepos([repoWithOwnPath])
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

/**
 * The mixed-split live bug (see ide-shell.test.tsx's "mixed split" describe
 * for the wiring half of this fix): a project-home chat's pane active while
 * the ROUTE sits on a repo workspace of the same project — `isHomeRoute` is
 * FALSE here, unlike every block above.
 *
 * This hook's own `sidebarWorkspaceId === homeWorkspaceId` branch
 * (use-ide-shell-workspace-retention.ts) never checked `isHomeRoute` — so it
 * already resolves correctly whenever `homeWorkspaceId`/`homeWorkspacePath`
 * are the REAL, matching ones. The bug was never in this hook: it was
 * `ide-shell.tsx` handing it `undefined`/`null` for both off the home route
 * (`homeProjectId = homeRouteMatch ? activeProjectIdFromRoute : undefined`).
 * This test locks in the hook's own half of the contract — that a correctly
 * resolved `homeWorkspaceId` is honored regardless of `isHomeRoute` — so a
 * future regression in the OTHER direction (re-adding an `isHomeRoute` check
 * to that branch) fails here too.
 */
describe('useIdeShellWorkspaceRetention — project-home pane, route on a DIFFERENT (repo) workspace of the same project', () => {
  afterEach(() => {
    destroyWorkspaceStore('ws-home-1')
  })

  it("resolves sidebarWorkspacePath to the home workspace's own path even though isHomeRoute is false", () => {
    useSidebarStore.getState().setRepos([repoWithOwnPath])
    const { paneActions } = windowPaneStore.getState()
    // The active pane's chat resolving to the home workspace via the
    // REGISTRY (not the sidebar hint, which can't name a home chat — see the
    // project-home-chat block above): a registered store is what the real
    // app has once the pane's own chat has streamed at least once, which is
    // exactly the "file explorer stuck empty, but the chat itself renders
    // fine" shape of the live bug.
    getOrCreateWorkspaceStore('ws-home-1').getState().upsertAgentChat({
      id: 'home-chat-1',
      workspaceId: 'ws-home-1',
      title: 'home-chat-1',
      liveRunnerId: '',
      terminalSessionId: '',
      activeProviderId: 'claude',
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
