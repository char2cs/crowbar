import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { Project } from '@/lib/types'
import { createLoadableSlice, type LoadableSlice } from '@/lib/store/loadable-slice'
import { fetchProjects } from '@/lib/api'

// Stable empty fallback for `useProjectDataStore((s) => dataOf(s.data) ?? EMPTY_PROJECTS)`
// selectors. Returning a fresh `[]` each render makes Zustand's useSyncExternalStore
// snapshot compare unstable (a new reference every call), which React treats as a
// perpetual change → "Maximum update depth exceeded". A shared constant keeps the
// reference identical across renders. Read-only by convention (consumers only
// map/find/filter).
export const EMPTY_PROJECTS: Project[] = []

interface ProjectState {
  projects: Project[]
  activeProjectId: string
  setActiveProject: (id: string) => void
  setProjects: (projects: Project[]) => void
  addProject: (project: Project) => void
}

// §6: the project list is GET-seeded and kept live by the `/v0/projects` WS
// stream — an imported/deleted project arrives as a ProjectDTO frame and the
// debounced applyDelta re-seeds the loadable (no caller-side double-refetch).
export const useProjectDataStore = create<LoadableSlice<Project[], []>>()((set, get) =>
  createLoadableSlice<Project[], []>({
    store: 'projects-data',
    fetcher: () => fetchProjects(),
    cacheKey: () => 'projects',
    wsEndpoint: () => '/v0/projects',
  })(set, get),
)

export const useProjectStore = create<ProjectState>()(
  persist(
    (set) => ({
      projects: [],
      activeProjectId: '',
      setActiveProject: (id) => set({ activeProjectId: id }),
      setProjects: (projects) => set({ projects }),
      addProject: (project) => set((s) => ({ projects: [...s.projects, project] })),
    }),
    { name: 'crowbar.activeProject', partialize: (s) => ({ activeProjectId: s.activeProjectId }) },
  ),
)

/**
 * Add an imported project to the live store so it appears immediately. The
 * canonical ProjectDTO (and its repos/workspaces) arrive over the `/v0/projects`
 * WS stream and the §7 per-repo entity streams, which re-seed the loadable and
 * the sidebar cache — so there is no caller-side double-refetch anymore (§6).
 */
export function importProjectAndSync(project: Project): void {
  useProjectStore.getState().addProject(project)
  void useProjectDataStore.getState().fetch()
}
