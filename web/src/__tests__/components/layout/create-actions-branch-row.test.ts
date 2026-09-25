/** A branch row is addressed by its owning chat, and every verb treats it as its workspace. */
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'

const { postWorkspace, createChat, createChatWithOwnWorktree, deleteChat } = vi.hoisted(() => ({
  postWorkspace: vi.fn(() => Promise.resolve()),
  createChat: vi.fn(() => Promise.resolve('chat-1')),
  createChatWithOwnWorktree: vi.fn(() => Promise.resolve('chat-1')),
  deleteChat: vi.fn(() => Promise.resolve()),
}))

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  postWorkspace,
}))
vi.mock('@/features/agent/api/agent-api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/agent/api/agent-api')>()),
  createChat,
  createChatWithOwnWorktree,
  deleteChat,
}))

import { resolveChatRow, resolveRow } from '@/components/layout/open-actions'
import {
  handleCreate,
  confirmPendingCreateName,
  cancelPendingCreate,
} from '@/components/layout/create-actions'
import { handleTrash } from '@/components/layout/trash-actions'
import { getInitialState, useSidebarStore, type Chat, type Repo } from '@/lib/store/sidebar'
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

/** A 'workspace' create now asks for a name before it fires (the pending
 *  row's inline input) — this drives that confirm for tests written against
 *  the old immediate-fire behavior, finding the single 'naming' entry
 *  `handleCreate` just armed exactly the way the real input would. */
function confirmArmedBranchName(name = 'typed-branch'): void {
  const armed = usePendingCreatesStore.getState().entries.find((e) => e.status === 'naming')
  if (!armed) throw new Error('confirmArmedBranchName: no naming entry is armed')
  confirmPendingCreateName(armed.tempId, name)
}

/**
 * A `branch` row's id is the id of the CHAT that owns its workspace
 * (`rows-from-repo.ts`), which puts it in the chat id space while making it no
 * chat at all. Every dispatcher here picks its behaviour by which space an id
 * falls in, so each one has to be able to tell the two apart — the bug this
 * closes is a locked branch's "+" going silently inert because `resolveChatRow`
 * matched its row and returned early.
 */
describe('a branch row is addressed by its owning chat, and is still a workspace', () => {
  const branchRow = (id: string, workspaceId: string): Chat => ({
    id,
    repoId: 'r1',
    ownsWorktree: true,
    workspaceId,
    title: '',
    order: 0,
  })

  const lockedRepo = () =>
    repo({
      workspaces: [
        {
          id: 'ws-locked',
          branch: 'develop',
          age: '',
          status: 'locked',
          owningChatId: 'develop-row',
        },
        { id: 'ws-open', branch: 'feature/x', age: '' },
      ],
      chats: [branchRow('home-row', 'home-1'), branchRow('develop-row', 'ws-locked')],
    })

  it('is not a chat row', () => {
    expect(resolveChatRow([lockedRepo()], 'develop-row')).toBeNull()
  })

  it('resolves to the WORKSPACE it draws, so drag and removal see one id space', () => {
    const found = resolveRow([lockedRepo()], 'develop-row')
    expect(found?.subject).toMatchObject({ kind: 'workspace', id: 'ws-locked', locked: true })
  })

  it('the repo-home row resolves to the default workspace', () => {
    expect(resolveRow([lockedRepo()], 'home-row')?.subject).toMatchObject({
      kind: 'workspace',
      id: 'home-1',
    })
  })

  it('its "+" creates a workspace under the OWNING CHAT id — the id the daemon places by', () => {
    useSidebarStore.setState({ repos: [lockedRepo()] })
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })

    handleCreate('develop-row', 'workspace', vi.fn())
    confirmArmedBranchName()

    expect(createChatWithOwnWorktree).toHaveBeenCalledExactlyOnceWith(
      'p1',
      'r1',
      'claude',
      'develop-row',
      'typed-branch',
    )
  })

  it('its thread "+" runs in the WORKSPACE, not in the row id', () => {
    useSidebarStore.setState({ repos: [lockedRepo()] })
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })

    handleCreate('develop-row', 'thread', vi.fn())

    expect(createChat).toHaveBeenCalledExactlyOnceWith(
      'ws-locked',
      'claude',
      'develop-row',
      undefined,
    )
  })

  describe('pending-create rows — placement and lifecycle', () => {
    it('arms a fork naming entry at the exact sibling slot the real row will land in, then clears once the real row lands', async () => {
      useAgentProvidersStore.setState({
        status: 'ready',
        providers: [{ id: 'claude', enabled: true }] as never,
      })
      useSidebarStore.setState({
        repos: [
          repo({
            workspaces: [
              { id: 'ws-a', branch: 'alpha', age: '', order: 0 },
              { id: 'ws-b', branch: 'beta', age: '', order: 1 },
            ],
          }),
        ],
      })

      handleCreate('home-1', 'workspace', vi.fn())

      const armed = usePendingCreatesStore.getState().entries
      expect(armed).toHaveLength(1)
      expect(armed[0]).toMatchObject({
        kind: 'branch',
        status: 'naming',
        projectId: 'p1',
        parentId: 'home-1',
        order: 2,
      })

      confirmArmedBranchName('feature/x')
      expect(usePendingCreatesStore.getState().entries[0]).toMatchObject({
        status: 'creating',
        label: 'feature/x',
      })
      await Promise.resolve()

      // Not cleared yet — the create's own promise resolved, but the real row
      // has not been OBSERVED in the store, which is the whole point of
      // `waitForRow`/`chatHasLanded` rather than clearing on the promise alone.
      expect(usePendingCreatesStore.getState().entries).toHaveLength(1)

      // Regression: a fork mints its owning chat CHAT-FIRST — the two land as
      // separate reseed frames, never atomically. The chat alone is not
      // "landed" for a fork the way it is for a thread: without its own
      // workspace record, rows-from-repo.ts has no fold to nest it by, so it
      // would render at the repo root — exactly the frame that must never
      // reach the screen. Still pending here, on purpose.
      useSidebarStore.setState({
        repos: [
          repo({
            workspaces: [
              { id: 'ws-a', branch: 'alpha', age: '', order: 0 },
              { id: 'ws-b', branch: 'beta', age: '', order: 1 },
            ],
            chats: [{ id: 'chat-1', repoId: 'r1', title: '', order: 0 }],
          }),
        ],
      })
      await Promise.resolve()

      expect(usePendingCreatesStore.getState().entries).toHaveLength(1)

      // Regression, reported live: a fork's PLACEMENT (Node, separate from its
      // mint) is its OWN second write too — the workspace-owner half landing
      // does not by itself prove the chat's own placement has. This reseed
      // shows both the chat AND its owning workspace landed, but the chat is
      // still parented at root (its own placement not yet caught up) — must
      // still stay pending, or the real (misplaced) row renders before
      // self-correcting a beat later.
      useSidebarStore.setState({
        repos: [
          repo({
            workspaces: [
              { id: 'ws-a', branch: 'alpha', age: '', order: 0 },
              { id: 'ws-b', branch: 'beta', age: '', order: 1 },
              { id: 'ws-c', branch: 'feature/x', age: '', order: 2, owningChatId: 'chat-1' },
            ],
            chats: [{ id: 'chat-1', repoId: 'r1', title: '', order: 0 }],
          }),
        ],
      })
      await Promise.resolve()

      expect(usePendingCreatesStore.getState().entries).toHaveLength(1)

      // The placement write catches up too — NOW every half is landed, and
      // clearing the pending row reveals the real one already correctly
      // folded/nested, never a beat at the root first.
      useSidebarStore.setState({
        repos: [
          repo({
            workspaces: [
              { id: 'ws-a', branch: 'alpha', age: '', order: 0 },
              { id: 'ws-b', branch: 'beta', age: '', order: 1 },
              { id: 'ws-c', branch: 'feature/x', age: '', order: 2, owningChatId: 'chat-1' },
            ],
            chats: [{ id: 'chat-1', repoId: 'r1', title: '', order: 0, parentId: 'home-1' }],
          }),
        ],
      })
      await Promise.resolve()

      expect(usePendingCreatesStore.getState().entries).toEqual([])
    })

    it('a thread create skips naming — goes straight to a spinner row at the next sibling slot', async () => {
      useAgentProvidersStore.setState({
        status: 'ready',
        providers: [{ id: 'claude', enabled: true }] as never,
      })
      useSidebarStore.setState({
        repos: [
          repo({
            workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }],
            chats: [{ id: 'c-existing', repoId: 'r1', title: 'first', order: 0, parentId: 'ws-a' }],
          }),
        ],
      })

      handleCreate('ws-a', 'thread', vi.fn())

      const armed = usePendingCreatesStore.getState().entries
      expect(armed).toHaveLength(1)
      expect(armed[0]).toMatchObject({
        kind: 'chat',
        status: 'creating',
        parentId: 'ws-a',
        order: 1,
        workspaceId: 'ws-a',
      })
      expect(createChat).toHaveBeenCalledExactlyOnceWith('ws-a', 'claude', 'ws-a', undefined)
    })

    it('cancelling a naming entry drops the row and releases the lock — a fresh "+" click arms again', () => {
      useAgentProvidersStore.setState({
        status: 'ready',
        providers: [{ id: 'claude', enabled: true }] as never,
      })
      useSidebarStore.setState({ repos: [repo()] })

      handleCreate('home-1', 'workspace', vi.fn())
      const firstTempId = usePendingCreatesStore.getState().entries[0]?.tempId
      expect(firstTempId).toBeDefined()

      cancelPendingCreate(firstTempId as string)
      expect(usePendingCreatesStore.getState().entries).toEqual([])

      handleCreate('home-1', 'workspace', vi.fn())
      expect(usePendingCreatesStore.getState().entries).toHaveLength(1)
      expect(createChatWithOwnWorktree).not.toHaveBeenCalled()
    })
  })

  it('its trash takes the WORKSPACE path — refused as locked, never deleteChat', () => {
    useSidebarStore.setState({ repos: [lockedRepo()] })

    // A branch row can only ever be a locked branch or a repo home, and
    // `planRemoval`'s `draftFor` refuses both — so the tray staying empty is
    // the REFUSAL, and on its own it is indistinguishable from doing nothing.
    // The ordinary workspace below is what tells those two apart: the same
    // call, in the same repo, does reach the tray.
    expect(handleTrash('develop-row')).toBe(false)
    expect(deleteChat).not.toHaveBeenCalled()
    expect(useRemovalTrayStore.getState().entries).toEqual([])

    expect(handleTrash('ws-open')).toBe(true)
    expect(useRemovalTrayStore.getState().entries).toHaveLength(1)
    expect(deleteChat).not.toHaveBeenCalled()
  })
})
