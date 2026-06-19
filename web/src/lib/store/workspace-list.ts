import { create } from 'zustand'
import { createLoadableSlice, type LoadableSlice } from '@/lib/store/loadable-slice'
import { getAllEntities } from '@/lib/persistence/entity-cache'
import type { Repo } from '@/lib/store/sidebar'
import { buildRepoTree, type RepoDTO, type WorkspaceDTO } from '@/lib/store/build-repo-tree'

// §6/§7: the sidebar tree is now derived from the WS-driven entity cache, not a
// flat cross-project GET. `fetch` reads the per-entity object stores
// (crowbar_repos + crowbar_workspaces) and groups them into the nested Repo[]
// the sidebar renders. The per-repo GET seed + live WS subscription that keep
// the cache fresh live in app-sync-provider's §7 startup (subscribeEntityStream
// over `/v0/projects/:p/repos/:r/workspaces`), so this store no longer owns a
// `/v0/ws/workspaces` subscription — `startSync` is a no-op and the live tree
// is refreshed by calling `fetch()` whenever the cache changes.
async function buildTreeFromCache(): Promise<Repo[]> {
  const [repos, workspaces] = await Promise.all([
    getAllEntities<RepoDTO>('crowbar_repos'),
    getAllEntities<WorkspaceDTO>('crowbar_workspaces'),
  ])
  return buildRepoTree(repos, workspaces)
}

export const useWorkspaceListStore = create<LoadableSlice<Repo[], []>>()((set, get) =>
  createLoadableSlice<Repo[], []>({
    store: 'workspaces-data',
    fetcher: buildTreeFromCache,
    cacheKey: () => 'workspaces',
  })(set, get),
)
