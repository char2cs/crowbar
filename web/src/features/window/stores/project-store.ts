// Stub: window/project feature is out of scope for this session.
import { create } from 'zustand'
import { createSelectors } from '@/utils/zustand-selectors'

export interface ProjectState {
  projectPath: string | null
  rootFolderPath: string | null
  projectName: string | null
  recentProjects: string[]
}

const useProjectStoreBase = create<ProjectState>(() => ({
  projectPath: null,
  rootFolderPath: null,
  projectName: null,
  recentProjects: [],
}))

export const useProjectStore = createSelectors(useProjectStoreBase)
