/**
 * K3 class (f), repo-internal half: the sidebar's repo rows are rebuilt from
 * the IndexedDB entity cache (`readVisibleRepoTree`), which iterates by key —
 * the chat id. `buildSidebarTree` breaks an `order` tie by arrival, so a
 * legacy level whose chats all still carry order 0 is drawn in ID order. The
 * daemon's chat writer breaks that same tie by CreatedAt (tree.compareNodes:
 * order, kind rank, createdAt, id), so a chat↔chat drag on such a level is
 * indexed against one sequence and applied against another: the row lands
 * beside a different sibling than the drop line promised and untouched rows
 * jump. Whatever the FE ties on has to be what the daemon ties on.
 */
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { IDBFactory } from 'fake-indexeddb'
import { resetDB } from '@/lib/persistence/idb'
import { upsertEntity } from '@/lib/persistence/entity-cache'
import { success } from '@/lib/loadable'
import { readVisibleRepoTree } from '@/lib/store/project-visibility'
import { rowsFromRepo } from '@/components/sidebar/lib/rows-from-repo'
import { useHomeTreeStore } from '@/lib/store/home-tree'
import { useProjectDataStore, useProjectStore } from '@/lib/store/projects'
import type { ChatDTO, Project, RepoDTO } from '@/lib/types'

vi.mock('@/features/workspace/lib/home-workspace-resolver', () => ({
  getHomeWorkspaceId: () => null,
  getHomeOwningChatId: () => null,
}))

const repoDTO: RepoDTO = {
  id: 'r1',
  projectId: 'p1',
  name: 'r1',
  path: '/p/r1',
  defaultBranch: 'main',
  avatarLabel: 'R',
  avatarColor: 'bg-indigo-700',
  avatarUrl: '',
  avatarEmoji: '',
  order: 0,
}

const project: Project = { id: 'p1', name: 'p1', path: '/p/p1', lastActivity: new Date(0) }

// The wire's AgentChatDTO carries `createdAt`; the cache stores the row as is.
const chatDTO = (id: string, createdAt: string): ChatDTO & { createdAt: string } => ({
  id,
  repoId: 'r1',
  projectId: 'p1',
  type: 'chat',
  workspaceId: '',
  ownsWorktree: false,
  parentId: '',
  title: id,
  order: 0,
  createdAt,
})

beforeEach(async () => {
  resetDB()
  globalThis.indexedDB = new IDBFactory()
  useHomeTreeStore.setState({ trees: {} })
  useProjectStore.setState({ activeProjectId: 'p1' })
  useProjectDataStore.setState({ data: success([project]) })
  await upsertEntity('crowbar_repos', repoDTO)
  // Created c-z first, then c-m, then c-a: id order is the reverse of creation.
  await upsertEntity('crowbar_chats', chatDTO('c-z', '2026-01-01T00:00:00Z'))
  await upsertEntity('crowbar_chats', chatDTO('c-m', '2026-01-02T00:00:00Z'))
  await upsertEntity('crowbar_chats', chatDTO('c-a', '2026-01-03T00:00:00Z'))
})

describe('a legacy all-zero chat level rebuilt from the entity cache', () => {
  it('draws tied chats in the order the daemon breaks the tie (createdAt), not by cache key', async () => {
    const repos = await readVisibleRepoTree()
    const repo = repos.find((r) => r.id === 'r1')
    expect(repo).toBeDefined()
    const chatRows = rowsFromRepo(repo!)
      .filter((row) => row.kind === 'chat')
      .map((row) => row.id)
    expect(chatRows).toEqual(['c-z', 'c-m', 'c-a'])
  })
})
