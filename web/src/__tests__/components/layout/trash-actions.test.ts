/** Trashing a sidebar row routes every kind through the removal tray. */
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'

const { postWorkspace, deleteChat, getHomeWorkspaceId } = vi.hoisted(() => ({
  postWorkspace: vi.fn(() => Promise.resolve()),
  deleteChat: vi.fn(() => Promise.resolve()),
  getHomeWorkspaceId: vi.fn(),
}))

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  postWorkspace,
}))
vi.mock('@/features/agent/api/agent-api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/agent/api/agent-api')>()),
  deleteChat,
}))
// `handleOpen`'s home branch reads this directly (see `resolveHomeRow`) —
// the real resolver needs an async fetch+cache round trip these tests have
// no reason to exercise; `handleCreateHomeThread`'s own tests never needed
// this mock since they take `homeWorkspaceId` as a direct argument instead.
vi.mock('@/features/workspace/lib/home-workspace-resolver', () => ({
  getHomeWorkspaceId,
  getHomeOwningChatId: () => null,
}))

import { handleTrash } from '@/components/layout/trash-actions'
import { getInitialState, useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { getInitialRemovalState, useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import { useSettingsStore } from '@/features/settings/store'
import { usePendingCreatesStore, getInitialPendingCreatesState } from '@/lib/store/pending-creates'
import { useHomeTreeStore } from '@/lib/store/home-tree'

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

afterEach(() => {
  // A GLOBAL store — a leaked 'terminal' default would silently arm every
  // later create in this file with a surface its own assertions never named.
  useSettingsStore.setState((state) => ({
    settings: { ...state.settings, chatIsDefaultPresentation: true },
  }))
})

beforeEach(() => {
  vi.clearAllMocks()
  useSidebarStore.setState(getInitialState())
  useRemovalTrayStore.setState(getInitialRemovalState())
  useHomeTreeStore.setState({ trees: {} })
  usePendingCreatesStore.setState(getInitialPendingCreatesState())
  // Create-workspace now needs a PROVIDER (the new atomic endpoint starts a
  // CLI, unlike the old chat-less postWorkspace) — the global provider store
  // (agent-providers-store.ts), not a per-workspace one, since there is no
  // workspace yet to scope a per-workspace read through.
  useAgentProvidersStore.setState({ status: 'ready', providers: [] })
})

describe('handleTrash', () => {
  it('holds a real workspace row in the removal tray, and reports it', () => {
    useSidebarStore.setState({
      repos: [repo({ workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }] })],
    })

    expect(handleTrash('ws-a')).toBe(true)

    expect(useRemovalTrayStore.getState().entries).toHaveLength(1)
    expect(useRemovalTrayStore.getState().entries[0]?.id).toBe('ws-a')
  })

  it('is a no-op for the repo-home row (no matching row for planRemoval to draft), and reports it', () => {
    useSidebarStore.setState({ repos: [repo()] })

    expect(handleTrash('home-1')).toBe(false)

    expect(useRemovalTrayStore.getState().entries).toEqual([])
  })

  // The literal live-caught bug: the daemon's ListInRepo never filters by
  // the repo id in its own URL (fetchFolders's own doc — a known, unfixed
  // backend leniency), so a home folder bleeds into every REPO's own
  // folders array too, stamped with THAT repo's id. `resolveRow`'s
  // repo-scoped walk found this FALSE match, and the removal tray then
  // committed a real DELETE against a repo that had no business resolving
  // it at all — silently destroying a home folder dragged onto the trash
  // target. `resolveHomeRowScope` must resolve a home row BEFORE that walk
  // ever runs, regardless of what a repo's own (bled-into) folders array
  // claims — proven here by the held draft's OWN `repoId`: '' (home), never
  // 'r1' (the bled repo the walk would have found instead).
  it('holds a home folder through the HOME path even when a repo’s (backend-leniency-bled) folders array also claims its id', () => {
    getHomeWorkspaceId.mockReturnValue('home-ws-1')
    useHomeTreeStore.setState({
      trees: {
        p1: { chats: [], folders: [{ id: 'home-folder-1', repoId: '', name: 'x', order: 0 }] },
      },
    })
    useSidebarStore.setState({
      repos: [repo({ folders: [{ id: 'home-folder-1', repoId: 'r1', name: 'x', order: 0 }] })],
    })

    expect(handleTrash('home-folder-1')).toBe(true)

    const entries = useRemovalTrayStore.getState().entries
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({
      kind: 'folder',
      id: 'home-folder-1',
      projectId: 'p1',
      repoId: '',
    })
  })

  // Task 25 review round 1, Important: a user-locked, non-home workspace
  // still shows a trash button (only the project-home row hides it), but
  // `draftFor` refuses to draft a locked workspace — the caller (the
  // delete-confirm dialog's onConfirm) needs this reported so it can tell
  // the user rather than silently swallowing a click it just walked them
  // through a confirmation for.
  it('is a no-op for a locked (non-home) workspace, and reports it', () => {
    useSidebarStore.setState({
      repos: [
        repo({
          workspaces: [
            { id: 'ws-locked', branch: 'locked-one', age: '', order: 0, status: 'locked' },
          ],
        }),
      ],
    })

    expect(handleTrash('ws-locked')).toBe(false)

    expect(useRemovalTrayStore.getState().entries).toEqual([])
  })

  // Addendum §2: a chat's delete now holds in the SAME removal tray every
  // other kind uses — `resolveChatRow` is still consulted before `resolveRow`
  // ever sees the id, but the outcome is a held `RemovalEntry`, not an
  // immediate `deleteChat` call.
  describe('a chat row', () => {
    it('holds a bubble chat in the removal tray, scoped through any workspace of its own repo', () => {
      useSidebarStore.setState({
        repos: [
          repo({
            defaultWorkspaceId: undefined,
            workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }],
            // No `workspaceId` — a bubble, not a worktree chat.
            chats: [{ id: 'c1', repoId: 'r1', title: 'a chat', order: 0 }],
          }),
        ],
      })

      expect(handleTrash('c1')).toBe(true)

      expect(deleteChat).not.toHaveBeenCalled()
      const entries = useRemovalTrayStore.getState().entries
      expect(entries).toHaveLength(1)
      expect(entries[0]).toMatchObject({ kind: 'chat', id: 'c1', wsId: 'ws-a' })
    })

    it('refuses when the repo has no workspace at all to scope the request through', () => {
      useSidebarStore.setState({
        repos: [
          repo({
            defaultWorkspaceId: undefined,
            workspaces: [],
            chats: [{ id: 'c1', repoId: 'r1', title: 'a chat', order: 0 }],
          }),
        ],
      })

      expect(handleTrash('c1')).toBe(false)

      expect(deleteChat).not.toHaveBeenCalled()
      expect(useRemovalTrayStore.getState().entries).toEqual([])
    })
  })
})
