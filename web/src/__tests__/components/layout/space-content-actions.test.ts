/**
 * Unit coverage for the handlers extracted from `sidebar-tree-panel.tsx`
 * (Task 8/29) into `space-content-actions.ts` (Task 30) so `SpaceScroller`
 * can share them across every project's panel. The logic itself is
 * unchanged — only its home moved — so these pin the same behavior the
 * panel's own (now-deleted) test file did, at the function level rather
 * than through a rendered tree.
 */
import { describe, expect, it, vi, beforeEach } from 'vitest'

const { postWorkspace, createChat, createChatWithOwnWorktree, deleteChat, toastError } = vi.hoisted(
  () => ({
    postWorkspace: vi.fn(() => Promise.resolve()),
    createChat: vi.fn(() => Promise.resolve('chat-1')),
    createChatWithOwnWorktree: vi.fn(() => Promise.resolve('chat-1')),
    deleteChat: vi.fn(() => Promise.resolve()),
    toastError: vi.fn(),
  }),
)

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
vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: toastError, success: vi.fn(), info: vi.fn() },
}))

import {
  resolveChatRow,
  resolveRow,
  handleOpen,
  handleTrash,
  handleCreate,
} from '@/components/layout/space-content-actions'
import { getInitialState, useSidebarStore, type Chat, type Repo } from '@/lib/store/sidebar'
import { getInitialRemovalState, useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'

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

beforeEach(() => {
  vi.clearAllMocks()
  useSidebarStore.setState(getInitialState())
  useRemovalTrayStore.setState(getInitialRemovalState())
  // Create-workspace now needs a PROVIDER (the new atomic endpoint starts a
  // CLI, unlike the old chat-less postWorkspace) — the global provider store
  // (agent-providers-store.ts), not a per-workspace one, since there is no
  // workspace yet to scope a per-workspace read through.
  useAgentProvidersStore.setState({ status: 'ready', providers: [] })
})

describe('resolveRow', () => {
  it('resolves a real workspace row', () => {
    const repos = [repo({ workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }] })]
    const found = resolveRow(repos, 'ws-a')
    expect(found?.repo.id).toBe('r1')
    expect(found?.subject).toEqual({
      kind: 'workspace',
      id: 'ws-a',
      repoId: 'r1',
      locked: false,
      parentId: undefined,
    })
  })

  it('resolves a folder row', () => {
    const repos = [repo({ folders: [{ id: 'f1', repoId: 'r1', name: 'spikes', order: 0 }] })]
    const found = resolveRow(repos, 'f1')
    expect(found?.subject).toEqual({ kind: 'folder', id: 'f1', repoId: 'r1', parentId: undefined })
  })

  it('resolves the repo-home id as a workspace subject with no matching row', () => {
    const repos = [repo()]
    const found = resolveRow(repos, 'home-1')
    expect(found?.subject).toEqual({ kind: 'workspace', id: 'home-1', repoId: 'r1' })
  })

  it('returns null for an id in no repo', () => {
    expect(resolveRow([repo()], 'nope')).toBeNull()
  })
})

describe('handleOpen', () => {
  it('navigates into a workspace row', () => {
    const navigate = vi.fn()
    const repos = [repo({ workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }] })]
    handleOpen('ws-a', repos, navigate)
    expect(navigate).toHaveBeenCalledWith({
      to: '/ide/$projectId/$repoId/$wsId',
      params: { projectId: 'p1', repoId: 'r1', wsId: 'ws-a' },
    })
  })

  it('toggles a folder instead of navigating', () => {
    const navigate = vi.fn()
    const toggle = vi.spyOn(useSidebarStore.getState(), 'toggleChatRow')
    const repos = [repo({ folders: [{ id: 'f1', repoId: 'r1', name: 'spikes', order: 0 }] })]
    handleOpen('f1', repos, navigate)
    expect(toggle).toHaveBeenCalledWith('f1')
    expect(navigate).not.toHaveBeenCalled()
  })

  it('is a no-op for a row not in the given (removal-filtered) repos', () => {
    const navigate = vi.fn()
    handleOpen('ghost', [repo()], navigate)
    expect(navigate).not.toHaveBeenCalled()
  })

  // Chat rows are drawn in the tree now (design spec §3.1's fourth row kind).
  // They looked exactly like every other row and did nothing at all, because
  // `resolveRow` only ever searched workspaces/folders/the repo home.
  describe('a chat row', () => {
    const withChat = (chat: Partial<Chat> & { id: string }) =>
      repo({
        workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }],
        chats: [{ repoId: 'r1', title: 'a chat', order: 0, ...chat }],
      })

    it('navigates to the workspace a WORKTREE chat owns, like a branch row', () => {
      const navigate = vi.fn()
      handleOpen('c1', [withChat({ id: 'c1', workspaceId: 'ws-a' })], navigate)
      expect(navigate).toHaveBeenCalledWith({
        to: '/ide/$projectId/$repoId/$wsId',
        params: { projectId: 'p1', repoId: 'r1', wsId: 'ws-a' },
      })
    })

    it('navigates to the repo home when that is the workspace it owns', () => {
      const navigate = vi.fn()
      handleOpen('c1', [withChat({ id: 'c1', workspaceId: 'home-1' })], navigate)
      expect(navigate).toHaveBeenCalledWith({
        to: '/ide/$projectId/$repoId/$wsId',
        params: { projectId: 'p1', repoId: 'r1', wsId: 'home-1' },
      })
    })

    it('folds a BUBBLE instead of navigating — it owns no workspace to open', () => {
      const navigate = vi.fn()
      const toggle = vi.spyOn(useSidebarStore.getState(), 'toggleChatRow')
      handleOpen('c1', [withChat({ id: 'c1' })], navigate)
      expect(toggle).toHaveBeenCalledWith('c1')
      expect(navigate).not.toHaveBeenCalled()
    })

    // Spec §9.2: a repo's chats are not a closed set, so a chat naming a
    // workspace outside this repo is ordinary. Routing to /ide/:p/:r/:ws with a
    // ws that is not under :r would be a URL nothing resolves.
    it('folds rather than routing to a workspace that is not in this repo', () => {
      const navigate = vi.fn()
      const toggle = vi.spyOn(useSidebarStore.getState(), 'toggleChatRow')
      handleOpen('c1', [withChat({ id: 'c1', workspaceId: 'ws-in-another-repo' })], navigate)
      expect(toggle).toHaveBeenCalledWith('c1')
      expect(navigate).not.toHaveBeenCalled()
    })
  })
})

/**
 * The two verbs a chat row must NOT answer wrongly.
 *
 * `handleCreate` used to fall through to a path built for a different row
 * kind and explain itself in that kind's words — worse than doing nothing,
 * because the explanation was false. `handleTrash` used to be the same
 * shape of bug fixed the other direction — a direct `deleteChat` call with
 * no removal-tray draft at all. Addendum §2 closes THAT gap instead: a chat
 * now goes through the exact same tray every other kind already did, so its
 * delete is no longer a special case.
 */
describe('a chat row does not borrow another row kind’s refusal', () => {
  const repoWithChat = () =>
    repo({ chats: [{ id: 'c1', repoId: 'r1', title: 'a chat', order: 0 }] })

  it('handleTrash holds a chat in the removal tray — no direct deleteChat call', () => {
    useSidebarStore.setState({ repos: [repoWithChat()] })

    expect(handleTrash('c1')).toBe(true)

    expect(deleteChat).not.toHaveBeenCalled()
    const entries = useRemovalTrayStore.getState().entries
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: 'chat', id: 'c1', label: 'a chat' })
    // repo()'s default `defaultWorkspaceId` ('home-1') is the scoped
    // workspace id the DELETE request is addressed through once the hold
    // actually commits — a repo with no real `workspaces` entries.
    expect(entries[0].wsId).toBe('home-1')
    // A chat drains on the same 8s clock every non-cascading kind uses —
    // it does not wait on Cancel/Remove the way a repo/project does.
    expect(entries[0].deadlineAt).not.toBeNull()
  })

  it('handleCreate is SILENT — never the folder’s "has none to run it in"', () => {
    useSidebarStore.setState({ repos: [repoWithChat()] })
    handleCreate('c1', 'thread')
    expect(toastError).not.toHaveBeenCalled()
    expect(createChat).not.toHaveBeenCalled()
    expect(postWorkspace).not.toHaveBeenCalled()
  })
})

// Task 8: "create workspace" now mints the workspace AND its first chat
// atomically (POST .../chats {ownWorktree: true}) instead of the old
// chat-less postWorkspace — a bare branch row today, with a separate child
// chat row only once something ELSE later starts a conversation in it. One
// call now produces both at once (model spec §4.1, "one command replaces
// every create path").
describe('creating a workspace off the repo-home row', () => {
  it('calls the atomic own-worktree endpoint, not postWorkspace', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    useSidebarStore.setState({ repos: [repo()] })

    handleCreate('home-1', 'workspace')
    await Promise.resolve()

    expect(createChatWithOwnWorktree).toHaveBeenCalledExactlyOnceWith(
      'p1',
      'r1',
      'claude',
      'home-1',
    )
    expect(postWorkspace).not.toHaveBeenCalled()
  })

  // The clicked row's own id is the fallback, not the rule — see the regular-fork
  // block below, where the workspace names a real owning chat to place by.
  it('falls back to the clicked row id for a workspace that names no owning chat', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    useSidebarStore.setState({
      repos: [repo({ workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }] })],
    })

    handleCreate('ws-a', 'workspace')
    await Promise.resolve()

    expect(createChatWithOwnWorktree).toHaveBeenCalledExactlyOnceWith('p1', 'r1', 'claude', 'ws-a')
  })

  it('picks the first ENABLED provider from the global provider store', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [
        { id: 'disabled-one', enabled: false },
        { id: 'codex', enabled: true },
      ] as never,
    })
    useSidebarStore.setState({ repos: [repo()] })

    handleCreate('home-1', 'workspace')
    await Promise.resolve()

    expect(createChatWithOwnWorktree).toHaveBeenCalledExactlyOnceWith('p1', 'r1', 'codex', 'home-1')
  })

  it('is a silent no-op with no enabled provider loaded yet', async () => {
    useAgentProvidersStore.setState({ status: 'ready', providers: [] })
    useSidebarStore.setState({ repos: [repo()] })

    handleCreate('home-1', 'workspace')
    await Promise.resolve()

    expect(createChatWithOwnWorktree).not.toHaveBeenCalled()
    expect(postWorkspace).not.toHaveBeenCalled()
  })
})

/**
 * A REGULAR fork is the one row whose id is NOT the id the daemon places by.
 * Its owning chat is `type: 'chat'` (`tree/backfill.go`'s `owningChatType`) and
 * is already drawn as its own conversation beside it, so the row cannot take
 * that id the way a locked branch's does — one id would land on two rows, one
 * of them its own parent. The workspace names it instead
 * (`WorkspaceDTO.owningChatId`), and the create reads it from there.
 */
describe('creating a workspace off a REGULAR fork row', () => {
  const forkRepo = () =>
    repo({
      workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0, owningChatId: 'c-owner' }],
      chats: [
        { id: 'c-owner', repoId: 'r1', type: 'chat', workspaceId: 'ws-a', title: '', order: 0 },
      ],
    })

  it('names the workspace’s OWNING CHAT, never the clicked row id', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    useSidebarStore.setState({ repos: [forkRepo()] })

    handleCreate('ws-a', 'workspace')
    await Promise.resolve()

    expect(createChatWithOwnWorktree).toHaveBeenCalledExactlyOnceWith(
      'p1',
      'r1',
      'claude',
      'c-owner',
    )
  })

  // The thread half is a different question with a different answer: it opens
  // that workspace's own store and posts to its chats mount, so it wants the
  // WORKSPACE and never a chat id.
  it('its thread "+" still runs in the workspace, not in the owning chat', () => {
    useSidebarStore.setState({ repos: [forkRepo()] })
    getOrCreateWorkspaceStore('ws-a').setState({
      agentChats: {
        ...getOrCreateWorkspaceStore('ws-a').getState().agentChats,
        providers: [{ id: 'claude', enabled: true }] as never,
      },
    })

    handleCreate('ws-a', 'thread')

    expect(createChat).toHaveBeenCalledExactlyOnceWith('ws-a', 'claude')
  })
})

describe('starting a thread on an empty folder', () => {
  it('says why instead of silently doing nothing', () => {
    useSidebarStore.setState({
      repos: [repo({ folders: [{ id: 'f1', repoId: 'r1', name: 'spikes', order: 0 }] })],
    })

    handleCreate('f1', 'thread')

    expect(toastError).toHaveBeenCalledOnce()
    expect(createChat).not.toHaveBeenCalled()
  })
})

describe('starting a thread on a real workspace', () => {
  it('creates a chat with the first enabled provider', () => {
    useSidebarStore.setState({
      repos: [repo({ workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }] })],
    })
    // Object-form `setState`, not a callback: the merged store's `setState`
    // type doesn't accept a void-returning immer callback here (a
    // pre-existing typing trap unrelated to this feature — see Task 13's own
    // report on the identical trap in pane-slice.test.ts).
    const store = getOrCreateWorkspaceStore('ws-a')
    store.setState({
      agentChats: {
        ...store.getState().agentChats,
        providers: [{ id: 'claude', enabled: true }] as never,
      },
    })

    handleCreate('ws-a', 'thread')

    expect(createChat).toHaveBeenCalledExactlyOnceWith('ws-a', 'claude')
  })
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
    type: 'branch',
    workspaceId,
    title: '',
    order: 0,
  })

  const lockedRepo = () =>
    repo({
      workspaces: [
        { id: 'ws-locked', branch: 'develop', age: '', status: 'locked' },
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

    handleCreate('develop-row', 'workspace')

    expect(createChatWithOwnWorktree).toHaveBeenCalledExactlyOnceWith(
      'p1',
      'r1',
      'claude',
      'develop-row',
    )
  })

  it('its thread "+" runs in the WORKSPACE, not in the row id', () => {
    useSidebarStore.setState({ repos: [lockedRepo()] })
    getOrCreateWorkspaceStore('ws-locked').setState({
      agentChats: {
        ...getOrCreateWorkspaceStore('ws-locked').getState().agentChats,
        providers: [{ id: 'claude', enabled: true }] as never,
      },
    })

    handleCreate('develop-row', 'thread')

    expect(createChat).toHaveBeenCalledExactlyOnceWith('ws-locked', 'claude')
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
