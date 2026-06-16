import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { Project } from '@/lib/types'
import { createLoadableSlice, type LoadableSlice } from '@/lib/store/loadable-slice'
import { fetchProjects } from '@/lib/api'
import { dataOf } from '@/lib/loadable'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { useSidebarStore } from '@/lib/store/sidebar'

interface ProjectState {
  projects: Project[]
  activeProjectId: string
  setActiveProject: (id: string) => void
  setProjects: (projects: Project[]) => void
  addProject: (project: Project) => void
}

export const useProjectDataStore = create<LoadableSlice<Project[], []>>()((set, get) =>
  createLoadableSlice<Project[], []>({
    store: 'projects-data',
    fetcher: () => fetchProjects(),
    cacheKey: () => 'projects',
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
 * Add an imported project to the live store, then refetch the projects +
 * workspace lists and merge the new repos into the sidebar tree (BUG-013).
 */
export function importProjectAndSync(project: Project): void {
  useProjectStore.getState().addProject(project)
  void useProjectDataStore.getState().fetch()
  void useWorkspaceListStore
    .getState()
    .fetch()
    .then(() => {
      const repos = dataOf(useWorkspaceListStore.getState().data)
      if (repos) useSidebarStore.getState().mergeRepos(repos)
    })
}
