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

/** Sidebar order, as the daemon sorts it (compareProjectDTOs). */
function byOrder(a: Project, b: Project): number {
  const d = (a.order ?? 0) - (b.order ?? 0)
  if (d !== 0) return d
  return a.id < b.id ? -1 : a.id > b.id ? 1 : 0
}

/**
 * One `/v0/projects` frame — a complete ProjectDTO, or a `status: 'deleted'`
 * tombstone — folded into the list. Every project write broadcasts every row
 * it changes, so the frames are the list and never need a re-read.
 */
function mergeProjectFrame(list: Project[], frame: unknown): Project[] | undefined {
  if (!frame || typeof frame !== 'object') return undefined
  const { status, ...project } = frame as Project & { status?: string }
  if (typeof project.id !== 'string') return undefined
  const index = list.findIndex((p) => p.id === project.id)
  if (status === 'deleted') return index === -1 ? list : list.filter((p) => p.id !== project.id)
  if (index !== -1 && JSON.stringify(list[index]) === JSON.stringify(project)) return list
  const next = index === -1 ? [...list, project] : list.map((p, i) => (i === index ? project : p))
  return next.sort(byOrder)
}

// §6: the project list is GET-seeded once and then kept live by the
// `/v0/projects` WS stream's frames alone — the snapshot it sends on subscribe
// merges as no-ops, so boot makes one request.
export const useProjectDataStore = create<LoadableSlice<Project[], []>>()((set, get) =>
  createLoadableSlice<Project[], []>({
    store: 'projects-data',
    fetcher: () => fetchProjects(),
    cacheKey: () => 'projects',
    wsEndpoint: () => '/v0/projects',
    mergeFrame: mergeProjectFrame,
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
