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
import { getInitialState, useSidebarStore, type Repo } from '@/lib/store/sidebar'
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
          workspaces: [{ id: 'ws-locked', branch: 'locked-one', age: '', order: 0, status: 'locked' }],
        }),
      ],
    })

    expect(handleTrash('ws-locked')).toBe(false)

    expect(useRemovalTrayStore.getState().entries).toEqual([])
  })
})
