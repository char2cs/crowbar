import { describe, expect, it, vi, beforeEach } from 'vitest'

vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/editor/stores/buffer-session-persistence', () => ({
  saveSessionToStore: vi.fn(),
  clearQueuedWorkspaceSessionSave: vi.fn(),
}))
const { toastError, apiFetch } = vi.hoisted(() => ({
  toastError: vi.fn(),
  apiFetch: vi.fn(() => Promise.resolve(undefined)),
}))
vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: toastError, info: vi.fn(), success: vi.fn() },
}))
vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  apiFetch,
}))
vi.mock('@/lib/api/sidebar-placement', () => ({
  placeWorkspace: vi.fn().mockResolvedValue(undefined),
  placeFolder: vi.fn(),
  placeHomeFolder: vi.fn(),
  placeRepo: vi.fn(),
}))
vi.mock('@/features/workspace/lib/home-workspace-resolver', () => ({
  getHomeWorkspaceId: () => null,
  getHomeOwningChatId: () => null,
}))

import { performSidebarDrop } from '@/components/sidebar/lib/drop-actions'
import { allowedModes } from '@/components/sidebar/lib/sidebar-drop-policy'
import { rowsFromRepo } from '@/components/sidebar/lib/rows-from-repo'
import { getInitialState, useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { __resetWorkspaceScopesForTest } from '@/lib/workspace-scope'

const repo: Repo = {
  id: 'r1',
  projectId: 'p1',
  name: 'repo',
  avatarLabel: 'R',
  avatarColor: 'c',
  defaultWorkspaceId: 'ws-main',
  defaultBranch: 'main',
  defaultOwningChatId: 'main-owner',
  workspaces: [
    {
      id: 'ws-a',
      branch: 'feat/a',
      age: '',
      order: 0,
      status: 'new',
      parentId: 'ws-main',
      owningChatId: '',
    },
    {
      id: 'ws-b',
      branch: 'release',
      age: '',
      order: 1,
      status: 'locked',
      parentId: 'ws-main',
      owningChatId: 'owner-b',
    },
  ],
  folders: [],
  chats: [],
}

beforeEach(() => {
  vi.clearAllMocks()
  __resetWorkspaceScopesForTest()
  useSidebarStore.setState(getInitialState())
  useSidebarStore.getState().setRepos([repo])
})

// A fork whose owning chat has not been recorded yet is addressed by its
// workspace id; the reparent a nesting drop fires is chat-keyed, so it can
// only throw — the matrix never offers it, and a drop that reaches the
// planner anyway says why instead of surfacing the scope registry error.
describe('dragging a chatless fork into another branch', () => {
  it('is not offered by the drop matrix — only a same-parent reorder is', () => {
    const rows = rowsFromRepo(repo)
    const subject = rows.find((r) => r.id === 'ws-a')!
    const target = rows.find((r) => r.id === 'owner-b')!

    expect(allowedModes([subject], target)).toEqual({ before: true, after: true, into: false })
  })

  it('says the chat is still loading instead of surfacing the scope registry error verbatim', async () => {
    const rows = rowsFromRepo(repo)
    const subject = rows.find((r) => r.id === 'ws-a')!
    const target = rows.find((r) => r.id === 'owner-b')!

    await performSidebarDrop([subject], target, 'into')

    expect(apiFetch).not.toHaveBeenCalled()
    expect(toastError).toHaveBeenCalledTimes(1)
    expect(toastError.mock.calls[0][0]).toBe("Can't move feat/a yet — its chat is still loading")
  })
})
