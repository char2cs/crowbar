import { create } from 'zustand'
import { createLoadableSlice, type LoadableSlice } from '@/lib/store/loadable-slice'
import { apiFetch } from '@/lib/api'
import type { Repo } from '@/lib/store/sidebar'
import { buildRepoTree, type RepoDTO, type WorkspaceDTO } from '@/lib/store/build-repo-tree'

// The backend serves repos and workspaces as two flat lists; the sidebar wants
// workspaces nested under their repo, so we fetch both and group them here.
async function fetchRepoTree(): Promise<Repo[]> {
  const [repos, workspaces] = await Promise.all([
    apiFetch<RepoDTO[]>('/v0/repos'),
    apiFetch<WorkspaceDTO[]>('/v0/workspaces'),
  ])
  return buildRepoTree(repos, workspaces)
}

export const useWorkspaceListStore = create<LoadableSlice<Repo[], []>>()((set, get) =>
  createLoadableSlice<Repo[], []>({
    store: 'workspaces-data',
    fetcher: fetchRepoTree,
    cacheKey: () => 'workspaces',
    wsEndpoint: () => '/v0/ws/workspaces',
  })(set, get),
)
