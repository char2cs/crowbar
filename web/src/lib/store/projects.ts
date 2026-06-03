import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { Project } from '@/lib/types'
import { createLoadableSlice, type LoadableSlice } from '@/lib/store/loadable-slice'
import { fetchProjects } from '@/lib/api'

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
      addProject: (project) => set(s => ({ projects: [...s.projects, project] })),
    }),
    { name: 'crowbar.activeProject', partialize: s => ({ activeProjectId: s.activeProjectId }) },
  ),
)
