import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { Project } from '@/lib/types'
import { getAllMockProjects } from '@/lib/mock/projects'

interface ProjectState {
  projects: Project[]
  activeProjectId: string
  setActiveProject: (id: string) => void
  addProject: (project: Project) => void
}

function getInitialState() {
  const projects = getAllMockProjects()
  return { projects, activeProjectId: projects[0]?.id ?? '' }
}

export const useProjectStore = create<ProjectState>()(
  persist(
    (set) => ({
      ...getInitialState(),
      setActiveProject: (id) => set({ activeProjectId: id }),
      addProject: (project) => set(s => ({ projects: [...s.projects, project] })),
    }),
    { name: 'crowbar.activeProject', partialize: s => ({ activeProjectId: s.activeProjectId }) },
  ),
)

;(useProjectStore as any).getInitialState = getInitialState
