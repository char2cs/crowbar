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
vi.mock('@/features/agent/api/agent-chat-commands', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/agent/api/agent-chat-commands')>()),
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
import type { Repo } from '@/core/entities/repo-types'
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import { useSettingsStore } from '@/features/settings/store'
import { resetEntities, seedRepos } from '@/__tests__/__fixtures__/sidebar-entities'

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
  resetEntities()
  treeUiStore.setState(treeUiStore.getInitialState(), true)
  resetEntities()
  // Create-workspace now needs a PROVIDER (the new atomic endpoint starts a
  // CLI, unlike the old chat-less postWorkspace) — the global provider store
  // (agent-providers-store.ts), not a per-workspace one, since there is no
  // workspace yet to scope a per-workspace read through.
  useAgentProvidersStore.setState({ status: 'ready', providers: [] })
})

describe('handleTrash', () => {
  it('holds a real workspace row in the removal tray, and reports it', () => {
    seedRepos([repo({ workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }] })])

    expect(handleTrash('ws-a')).toBe(true)

    expect(treeUiStore.getState().removals).toHaveLength(1)
    expect(treeUiStore.getState().removals[0]?.id).toBe('ws-a')
  })

  it('is a no-op for the repo-home row (no matching row for planRemoval to draft), and reports it', () => {
    seedRepos([repo()])

    expect(handleTrash('home-1')).toBe(false)

    expect(treeUiStore.getState().removals).toEqual([])
  })

  // Task 25 review round 1, Important: a user-locked, non-home workspace
  // still shows a trash button (only the project-home row hides it), but
  // `draftFor` refuses to draft a locked workspace — the caller (the
  // delete-confirm dialog's onConfirm) needs this reported so it can tell
  // the user rather than silently swallowing a click it just walked them
  // through a confirmation for.
  it('is a no-op for a locked (non-home) workspace, and reports it', () => {
    seedRepos([
      repo({
        workspaces: [
          { id: 'ws-locked', branch: 'locked-one', age: '', order: 0, status: 'locked' },
        ],
      }),
    ])

    expect(handleTrash('ws-locked')).toBe(false)

    expect(treeUiStore.getState().removals).toEqual([])
  })

  // Addendum §2: a chat's delete now holds in the SAME removal tray every
  // other kind uses — `resolveChatRow` is still consulted before `resolveRow`
  // ever sees the id, but the outcome is a held `RemovalEntry`, not an
  // immediate `deleteChat` call.
  describe('a chat row', () => {
    it('holds a bubble chat in the removal tray, scoped through any workspace of its own repo', () => {
      seedRepos([
        repo({
          defaultWorkspaceId: undefined,
          workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }],
          // No `workspaceId` — a bubble, not a worktree chat.
          chats: [{ id: 'c1', repoId: 'r1', title: 'a chat', order: 0 }],
        }),
      ])

      expect(handleTrash('c1')).toBe(true)

      expect(deleteChat).not.toHaveBeenCalled()
      const entries = treeUiStore.getState().removals
      expect(entries).toHaveLength(1)
      expect(entries[0]).toMatchObject({ kind: 'chat', id: 'c1', wsId: 'ws-a' })
    })

    it('refuses when the repo has no workspace at all to scope the request through', () => {
      seedRepos([
        repo({
          defaultWorkspaceId: undefined,
          workspaces: [],
          chats: [{ id: 'c1', repoId: 'r1', title: 'a chat', order: 0 }],
        }),
      ])

      expect(handleTrash('c1')).toBe(false)

      expect(deleteChat).not.toHaveBeenCalled()
      expect(treeUiStore.getState().removals).toEqual([])
    })
  })
})
