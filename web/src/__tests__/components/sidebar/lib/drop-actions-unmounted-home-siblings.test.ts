import { describe, expect, it, vi, beforeEach } from 'vitest'

// REGRESSION (restyle v2, K3): a home chat dropped before/after ANOTHER home
// chat plans its `order` off `getOrCreateWorkspaceStore(homeWs).agentChats`
// (drop-actions.ts `planChatDrop`) — the per-WORKSPACE chat list that only
// `use-workspace-agent-chats-stream.ts` fills, and only while that workspace
// is MOUNTED. The sidebar draws every project's home rows off
// `useHomeTreeStore` regardless of what is mounted, so the drop reads an
// empty sibling list for any home whose pane is not open (the common case:
// you are in a repo workspace), `insertIndex([], target)` clamps to 0, and
// every home-chat reorder writes `order: 0` — the row never moves, or jumps
// to the top. (`getOrCreateWorkspaceStore` also MINTS a leaked store for the
// never-opened workspace on the way.) A chat dropped relative to a REPO
// header goes through `planChatDropOntoBranch`, which reads the rendered
// `rowsFromHome` rows and gets the right index — the two halves of the same
// level disagree on where the siblings live.
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
vi.mock('@/features/agent/api/agent-api', () => ({
  setChatPlacement: vi.fn(
    async (workspaceId: string, chatId: string, patch: { parentId?: string; order?: number }) => ({
      chat: { id: chatId, workspaceId, parentId: patch.parentId ?? '', order: patch.order ?? 0 },
      shifted: [],
    }),
  ),
}))
const { getHomeWorkspaceId } = vi.hoisted(() => ({ getHomeWorkspaceId: vi.fn() }))
vi.mock('@/features/workspace/lib/home-workspace-resolver', () => ({
  getHomeWorkspaceId,
  getHomeOwningChatId: () => null,
}))

import { performSidebarDrop } from '@/components/sidebar/lib/drop-actions'
import { setChatPlacement } from '@/features/agent/api/agent-api'
import { useHomeTreeStore } from '@/lib/store/home-tree'
import { getInitialState, useSidebarStore } from '@/lib/store/sidebar'
import { getInitialRemovalState, useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import { getAllActiveWorkspaceIds } from '@/features/workspace/stores/workspace-store-registry'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

const homeRow = (id: string, order: number): SidebarRow => ({
  id,
  kind: 'chat',
  parentId: null,
  order,
  label: id,
  ownsWorktree: false,
  workspaceId: 'home-ws-1',
  working: false,
  hasView: false,
})

beforeEach(() => {
  vi.clearAllMocks()
  useSidebarStore.setState(getInitialState())
  useRemovalTrayStore.setState(getInitialRemovalState())
  getHomeWorkspaceId.mockReturnValue('home-ws-1')
  // Three home chats, as the sidebar renders them — and NO workspace store
  // for home: its pane is not open.
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
          {
            id: 'a',
            repoId: '',
            ownsWorktree: false,
            workspaceId: 'home-ws-1',
            title: 'a',
            order: 0,
          },
          {
            id: 'b',
            repoId: '',
            ownsWorktree: false,
            workspaceId: 'home-ws-1',
            title: 'b',
            order: 1,
          },
          {
            id: 'c',
            repoId: '',
            ownsWorktree: false,
            workspaceId: 'home-ws-1',
            title: 'c',
            order: 2,
          },
        ],
      },
    },
  })
})

describe('reordering a home chat past another home chat while home is not mounted', () => {
  it('plans the index off the rendered home tree, not an empty per-workspace store', async () => {
    expect(getAllActiveWorkspaceIds()).not.toContain('home-ws-1')

    await performSidebarDrop([homeRow('a', 0)], homeRow('c', 2), 'after')

    // a lifted out of [a, b, c] -> [b, c]; after c -> index 2.
    expect(setChatPlacement).toHaveBeenCalledWith('home-ws-1', 'a', { parentId: '', order: 2 })
  })
})
