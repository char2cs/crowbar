/**
 * Unit coverage for the handlers extracted from `sidebar-tree-panel.tsx`
 * (Task 8/29) into `space-content-actions.ts` (Task 30) so `SpaceScroller`
 * can share them across every project's panel. The logic itself is
 * unchanged — only its home moved — so these pin the same behavior the
 * panel's own (now-deleted) test file did, at the function level rather
 * than through a rendered tree.
 */
import { describe, expect, it, vi, beforeEach } from 'vitest'

const { postWorkspace, createChat, toastError } = vi.hoisted(() => ({
  postWorkspace: vi.fn(() => Promise.resolve()),
  createChat: vi.fn(() => Promise.resolve('chat-1')),
  toastError: vi.fn(),
}))

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  postWorkspace,
}))
vi.mock('@/features/agent/api/agent-api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/agent/api/agent-api')>()),
  createChat,
}))
vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: toastError, success: vi.fn(), info: vi.fn() },
}))

import {
  resolveRow,
  handleOpen,
  handleTrash,
  handleCreate,
} from '@/components/layout/space-content-actions'
import { getInitialState, useSidebarStore, type Chat, type Repo } from '@/lib/store/sidebar'
import { getInitialRemovalState, useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'

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
 * Both used to fall through to a path built for a different row kind and
 * explain themselves in that kind's words — which is worse than doing nothing,
 * because the explanation was false.
 */
describe('a chat row does not borrow another row kind’s refusal', () => {
  const repoWithChat = () =>
    repo({ chats: [{ id: 'c1', repoId: 'r1', title: 'a chat', order: 0 }] })

  it('handleTrash refuses without claiming the chat is locked', () => {
    useSidebarStore.setState({ repos: [repoWithChat()] })
    // False, so the caller says "can't delete" rather than nothing — but it
    // must never have reached the workspace path that blames a lock. Nothing
    // is held either way.
    expect(handleTrash('c1')).toBe(false)
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

describe('creating a workspace off the repo-home row', () => {
  it('posts with the repo-home id as parentId, never an empty placement', async () => {
    useSidebarStore.setState({ repos: [repo()] })

    handleCreate('home-1', 'workspace')
    await Promise.resolve()

    expect(postWorkspace).toHaveBeenCalledExactlyOnceWith(
      'p1',
      'r1',
      expect.stringMatching(/^workspace-/),
      { parentId: 'home-1' },
    )
  })

  it('posts a real fork parent for a non-home workspace row too', async () => {
    useSidebarStore.setState({
      repos: [repo({ workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }] })],
    })

    handleCreate('ws-a', 'workspace')
    await Promise.resolve()

    expect(postWorkspace).toHaveBeenCalledExactlyOnceWith(
      'p1',
      'r1',
      expect.stringMatching(/^workspace-/),
      { parentId: 'ws-a' },
    )
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
})
