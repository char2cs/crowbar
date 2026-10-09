import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { Project } from '@/lib/types'
import { createLoadableSlice, type LoadableSlice } from '@/lib/store/loadable-slice'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { getHostRuntime } from '@/features/hosts/stores/host-runtime-registry'
import { LOCAL_HOST } from '@/features/hosts/types'

/**
 * How the list reaches the daemons. Host resolution reads this store and the
 * transport builds its URLs through host resolution, so the transport is handed
 * in at boot (main.tsx) rather than imported: importing it was an import cycle.
 */
export interface ProjectListTransport {
  /** `hostId`'s projects, each tagged with that host. */
  read: (hostId: string) => Promise<Project[]>
  /** The local daemon's `/v0/projects` frames; returns the unsubscribe. */
  subscribe: (onFrame: (frame: unknown) => void) => () => void
}

let transport: ProjectListTransport | null = null

/** Wires the list to the daemons, once, before anything reads it. */
export function connectProjectList(next: ProjectListTransport): void {
  transport = next
}

function connected(): ProjectListTransport {
  if (!transport) throw new Error('the project list has no transport: connectProjectList first')
  return transport
}

// Stable empty fallback for `useProjectDataStore((s) => dataOf(s.data) ?? EMPTY_PROJECTS)`
// selectors. Returning a fresh `[]` each render makes Zustand's useSyncExternalStore
// snapshot compare unstable (a new reference every call), which React treats as a
// perpetual change → "Maximum update depth exceeded". A shared constant keeps the
// reference identical across renders. Read-only by convention (consumers only
// map/find/filter).
export const EMPTY_PROJECTS: Project[] = []

interface ProjectState {
  activeProjectId: string
  setActiveProject: (id: string) => void
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
 * it changes, so the frames are the list and never need a re-read. A wire frame
 * names no host, so it takes its socket's; an imported Project keeps its own.
 */
function mergeProjectFrame(
  list: Project[],
  frame: unknown,
  socketHostId: string,
): Project[] | undefined {
  if (!frame || typeof frame !== 'object') return undefined
  const { status, ...fields } = frame as Partial<Project> & { id?: unknown; status?: string }
  if (typeof fields.id !== 'string') return undefined
  const project = { ...fields, hostId: fields.hostId ?? socketHostId } as Project
  const index = list.findIndex((p) => p.id === project.id)
  if (status === 'deleted') return index === -1 ? list : list.filter((p) => p.id !== project.id)
  if (index !== -1 && JSON.stringify(list[index]) === JSON.stringify(project)) return list
  const next = index === -1 ? [...list, project] : list.map((p, i) => (i === index ? project : p))
  return next.sort(byOrder)
}

/**
 * Every project on the local daemon and on each paired host that is up. A
 * remote host failing drops only its own projects; the local daemon failing
 * fails the read, since an empty list would read as "no projects at all".
 */
async function readEveryHost(): Promise<Project[]> {
  const { read } = connected()
  const remotes = useHostConfigStore
    .getState()
    .hosts.filter((host) => getHostRuntime(host.id)?.getState().status === 'up')
  const [local, ...remote] = await Promise.allSettled([
    read(LOCAL_HOST.id),
    ...remotes.map((host) => read(host.id)),
  ])
  if (local.status === 'rejected') throw local.reason
  return [local.value, ...remote.map((r) => (r.status === 'fulfilled' ? r.value : []))].flat()
}

// §6: the project list is GET-seeded once and then kept live by the
// `/v0/projects` WS stream's frames alone — the snapshot it sends on subscribe
// merges as no-ops, so boot makes one request.
export const useProjectDataStore = create<LoadableSlice<Project[], []>>()((set, get) =>
  createLoadableSlice<Project[], []>({
    store: 'projects-data',
    fetcher: readEveryHost,
    // Not 'projects': a list cached before projects carried a host would hand
    // out rows with no hostId to route by.
    cacheKey: () => 'projects-by-host',
    subscribe: (onFrame) => connected().subscribe(onFrame),
    // This socket is the local daemon's.
    mergeFrame: (list, frame) => mergeProjectFrame(list, frame, LOCAL_HOST.id),
  })(set, get),
)

export const useProjectStore = create<ProjectState>()(
  persist(
    (set) => ({
      activeProjectId: '',
      setActiveProject: (id) => set({ activeProjectId: id }),
    }),
    { name: 'crowbar.activeProject', partialize: (s) => ({ activeProjectId: s.activeProjectId }) },
  ),
)

/**
 * Fold an imported project into the live list the moment its import answers:
 * the root route reads that list next, and the stream's own frame for the
 * import may still be on its way.
 */
export function importProjectAndSync(project: Project): void {
  void useProjectDataStore.getState().applyDelta(project)
}
