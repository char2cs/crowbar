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
 * because the explanation was false. `handleTrash` is the same shape of bug
 * fixed the other direction: a chat delete must never fall into the
 * removal-tray/workspace-lock path, even now that it succeeds for real.
 */
describe('a chat row does not borrow another row kind’s refusal', () => {
  const repoWithChat = () =>
    repo({ chats: [{ id: 'c1', repoId: 'r1', title: 'a chat', order: 0 }] })

  it('handleTrash deletes directly — it never enters the removal tray', async () => {
    useSidebarStore.setState({ repos: [repoWithChat()] })
    expect(handleTrash('c1')).toBe(true)
    await Promise.resolve()
    // repo()'s default `defaultWorkspaceId` ('home-1') is the scoped
    // workspace id `scopedWorkspaceIdOf` resolves for a repo with no real
    // `workspaces` entries.
    expect(deleteChat).toHaveBeenCalledExactlyOnceWith('home-1', 'c1')
    expect(useRemovalTrayStore.getState().entries).toEqual([])
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

  it('names the clicked row as parentId for a non-home workspace row too', async () => {
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

  // A chat's delete is a direct `deleteChat` call, not a removal-tray draft —
  // `resolveChatRow` is consulted before `resolveRow` ever sees the id.
  describe('a chat row', () => {
    it('deletes a bubble chat, scoped through any workspace of its own repo', async () => {
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
      await Promise.resolve()

      expect(deleteChat).toHaveBeenCalledExactlyOnceWith('ws-a', 'c1')
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
      workspaces: [{ id: 'ws-locked', branch: 'develop', age: '', status: 'locked' }],
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

  it('its trash goes to the removal tray, never deleteChat', () => {
    useSidebarStore.setState({ repos: [lockedRepo()] })

    handleTrash('develop-row')

    expect(deleteChat).not.toHaveBeenCalled()
  })
})
