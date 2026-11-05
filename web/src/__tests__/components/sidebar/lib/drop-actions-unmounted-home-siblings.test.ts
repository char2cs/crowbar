import { describe, expect, it, vi, beforeEach } from 'vitest'
import { stubDropBoundary } from '@/__tests__/__fixtures__/drop-boundary'
import { getHomeWorkspaceId } from '@/features/workspace/lib/home-workspace-resolver'

// A home chat dropped beside another home chat plans its `order` off the
// rendered home tree, which is drawn whether or not that workspace is mounted.

import { performSidebarDrop } from '@/components/sidebar/lib/drop-actions'
import { setChatPlacement } from '@/features/agent/api/agent-chat-commands'
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'
import { getAllActiveWorkspaceIds } from '@/features/workspace/stores/workspace-store-registry'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { resetEntities, seedHomeTree } from '@/__tests__/__fixtures__/sidebar-entities'

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
  stubDropBoundary()
  resetEntities()
  treeUiStore.setState(treeUiStore.getInitialState(), true)
  vi.mocked(getHomeWorkspaceId).mockReturnValue('home-ws-1')
  // Three home chats, as the sidebar renders them — and NO workspace store
  // for home: its pane is not open.
  seedHomeTree('proj-1', {
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
