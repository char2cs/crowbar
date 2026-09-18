import { describe, expect, it, vi, beforeEach } from 'vitest'

// REGRESSION (restyle v2, K3): a repo header row is RENDERED with the id of
// the chat that owns its default workspace (`rows-from-repo.ts`
// `resolveHomeOwnerId`: `defaultOwningChatId || defaultWorkspaceId`), but
// `projectHomeContainerSiblings` (drop-actions.ts) lists every sibling repo
// by `defaultWorkspaceId`. For any repo whose main checkout has ever been
// chatted in, the two ids differ, so a repo dropped before/after ANOTHER
// repo's header never finds its target in `rest` and `insertIndex` clamps to
// the end: the PATCH goes out (204) with the wrong index and the row does not
// move. Repo-vs-home-chat drops are unaffected (a chat's row id IS its chat
// id), which is why the existing suite — whose repos carry no
// `defaultOwningChatId` — never saw it.
vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/editor/stores/buffer-session-persistence', () => ({
  saveSessionToStore: vi.fn(),
  clearQueuedWorkspaceSessionSave: vi.fn(),
}))
vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: vi.fn(), info: vi.fn(), success: vi.fn() },
}))
vi.mock('@/lib/api/sidebar-placement', () => ({
  placeWorkspace: vi.fn(),
  placeFolder: vi.fn(),
  placeHomeFolder: vi.fn(),
  placeRepo: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/lib/api/workspace', () => ({ reparentWorkspace: vi.fn() }))
vi.mock('@/features/agent/api/agent-api', () => ({ setChatPlacement: vi.fn() }))
const { getHomeWorkspaceId } = vi.hoisted(() => ({ getHomeWorkspaceId: vi.fn() }))
vi.mock('@/features/workspace/lib/home-workspace-resolver', () => ({
  getHomeWorkspaceId,
  getHomeOwningChatId: () => null,
}))

import { performSidebarDrop } from '@/components/sidebar/lib/drop-actions'
import { rowsFromRepo } from '@/components/sidebar/lib/rows-from-repo'
import { placeRepo } from '@/lib/api/sidebar-placement'
import { useHomeTreeStore } from '@/lib/store/home-tree'
import { getInitialState, useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { getInitialRemovalState, useRemovalTrayStore } from '@/lib/store/sidebar-removal'

const repo = (n: number): Repo => ({
  id: `repo-${n}`,
  projectId: 'proj-1',
  name: `repo-${n}`,
  avatarLabel: 'R',
  avatarColor: 'bg-indigo-700',
  order: n - 1,
  folderId: '',
  defaultWorkspaceId: `ws-main-${n}`,
  defaultBranch: 'main',
  // The main checkout has been chatted in: its row is id'd by that chat.
  defaultOwningChatId: `chat-main-${n}`,
  workspaces: [],
})

beforeEach(() => {
  vi.clearAllMocks()
  useSidebarStore.setState({ ...getInitialState(), repos: [repo(1), repo(2), repo(3)] })
  useRemovalTrayStore.setState(getInitialRemovalState())
  getHomeWorkspaceId.mockReturnValue('home-ws-1')
  useHomeTreeStore.setState({
    trees: {
      'proj-1': {
        folders: [],
        chats: [
          {
            id: 'home-owner',
            repoId: '',
            ownsWorktree: true,
            workspaceId: 'home-ws-1',
            title: '',
            order: 0,
          },
        ],
      },
    },
  })
})

describe('a repo header dropped relative to ANOTHER repo header', () => {
  it('indexes by the rendered header row id (its owning chat), not the raw workspace id', async () => {
    const header = (n: number) => rowsFromRepo(repo(n))[0]
    expect(header(2).id).toBe('chat-main-2')

    // [repo-1, repo-2, repo-3]: drag repo-3 BEFORE repo-2 -> rest = [1, 2],
    // index of repo-2 is 1.
    await performSidebarDrop([header(3)], header(2), 'before')

    expect(placeRepo).toHaveBeenCalledWith('proj-1', 'repo-3', { folderId: '', order: 1 })
  })
})
