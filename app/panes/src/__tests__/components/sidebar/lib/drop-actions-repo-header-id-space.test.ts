import { describe, expect, it, vi, beforeEach } from 'vitest'
import { stubDropBoundary } from '@/__tests__/__fixtures__/drop-boundary'
import { getHomeWorkspaceId } from '@/features/workspace/lib/home-workspace-resolver'

// A repo header row is rendered with the id of the chat owning its default workspace, so a
// header-to-header drop must index siblings by that row id, not by `defaultWorkspaceId`.

import { performSidebarDrop } from '@/components/sidebar/lib/drop-actions'
import { repoRows } from '@/__tests__/__fixtures__/tree-rows'
import { placeRepo } from '@/lib/api/sidebar-placement'
import type { Repo } from '@/core/entities/repo-types'
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'
import { seedHomeTree, seedRepos } from '@/__tests__/__fixtures__/sidebar-entities'
import { readSelect, selectRepos } from '@/core/entities/select'

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
  stubDropBoundary()
  seedRepos([repo(1), repo(2), repo(3)])
  treeUiStore.setState(treeUiStore.getInitialState(), true)
  vi.mocked(getHomeWorkspaceId).mockReturnValue('home-ws-1')
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
    ],
  })
})

describe('a repo header dropped relative to ANOTHER repo header', () => {
  it('indexes by the rendered header row id (its owning chat), not the raw workspace id', async () => {
    const header = (n: number) => repoRows(repo(n))[0]
    expect(header(2).id).toBe('chat-main-2')

    // [repo-1, repo-2, repo-3]: drag repo-3 BEFORE repo-2 -> rest = [1, 2],
    // index of repo-2 is 1.
    await performSidebarDrop([header(3)], header(2), 'before')

    expect(placeRepo).toHaveBeenCalledWith('proj-1', 'repo-3', { folderId: '', order: 1 })
  })
})

// After any sidebar drop, the order equals the daemon's answer — a
// refused reorder never paints, so there is nothing to revert.
describe('a repo reorder the daemon refuses', () => {
  it('leaves the headers exactly where they were', async () => {
    vi.mocked(placeRepo).mockRejectedValueOnce(new Error('409 conflict'))
    const before = readSelect(selectRepos).map((r) => [r.id, r.order])
    const header = (n: number) => repoRows(repo(n))[0]

    await performSidebarDrop([header(3)], header(1), 'before')

    expect(readSelect(selectRepos).map((r) => [r.id, r.order])).toEqual(before)
  })
})
