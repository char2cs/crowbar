import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { Project } from '@/lib/types'
import { dataOf } from '@/lib/loadable'
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

/** Local first, then each paired host in the order it was paired; unknown hosts last. */
function hostRank(hostId: string): number {
  if (hostId === LOCAL_HOST.id) return -1
  const index = useHostConfigStore.getState().hosts.findIndex((host) => host.id === hostId)
  return index === -1 ? Number.MAX_SAFE_INTEGER : index
}

/** Sidebar order: grouped by host (each daemon numbers only its own projects), then as
 *  that daemon sorts them (compareProjectDTOs). */
function byOrder(a: Project, b: Project): number {
  const h = hostRank(a.hostId) - hostRank(b.hostId)
  if (h !== 0) return h
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

/** One paired host's rows, read on their own: they replace that host's rows and no others. */
class HostRows {
  constructor(
    readonly hostId: string,
    readonly rows: Project[],
  ) {}
}

function isConfigured(hostId: string): boolean {
  return useHostConfigStore.getState().hosts.some((host) => host.id === hostId)
}

/** `list` with `hostId`'s rows swapped for `rows` — `list` itself when they match. */
function withHostRows(list: Project[], hostId: string, rows: Project[]): Project[] {
  const current = list.filter((p) => p.hostId === hostId)
  if (JSON.stringify(current) === JSON.stringify(rows)) return list
  return [...list.filter((p) => p.hostId !== hostId), ...rows].sort(byOrder)
}

/**
 * A local answer plus the held rows of every configured host: a host that is down,
 * slow or not probed yet keeps its projects until a read of its own replaces them.
 * Rows are never invented for a host nobody configured.
 */
function withHeldRemoteRows(local: Project[], held: Project[] | undefined): Project[] {
  const remote = (held ?? []).filter((p) => p.hostId !== LOCAL_HOST.id && isConfigured(p.hostId))
  return remote.length === 0 ? local : [...local, ...remote].sort(byOrder)
}

/** Per host, the newest read issued: an older answer that lands later is dropped. */
const hostReads = new Map<string, number>()

/**
 * Reads one paired host's projects and merges them in. A host that does not answer
 * keeps the rows it has: whether it is down is the prober's to say, not this read's.
 */
async function readHostRows(hostId: string): Promise<void> {
  const issued = (hostReads.get(hostId) ?? 0) + 1
  hostReads.set(hostId, issued)
  const rows = await connected()
    .read(hostId)
    .catch(() => null)
  if (rows === null || hostReads.get(hostId) !== issued || !isConfigured(hostId)) return
  await useProjectDataStore.getState().applyDelta(new HostRows(hostId, rows))
}

function upHostIds(): string[] {
  const up: string[] = []
  for (const host of useHostConfigStore.getState().hosts) {
    if (getHostRuntime(host.id)?.getState().status === 'up') up.push(host.id)
  }
  return up
}

// §6: the project list is GET-seeded once and then kept live by the
// `/v0/projects` WS stream's frames alone — the snapshot it sends on subscribe
// merges as no-ops, so boot makes one request.
export const useProjectDataStore = create<LoadableSlice<Project[], []>>()((set, get) =>
  createLoadableSlice<Project[], []>({
    store: 'projects-data',
    // The local daemon's answer is published the moment it lands, with each paired host's
    // held rows; a host that is up merges in when it answers. Its reads start only after
    // a local success, so a failing local read never re-arms itself through them.
    fetcher: async () => {
      const local = await connected().read(LOCAL_HOST.id)
      for (const hostId of upHostIds()) void readHostRows(hostId)
      return withHeldRemoteRows(local, dataOf(get().data))
    },
    // Not 'projects': a list cached before projects carried a host would hand
    // out rows with no hostId to route by.
    cacheKey: () => 'projects-by-host',
    subscribe: (onFrame) => connected().subscribe(onFrame),
    mergeFrame: (list, frame) => {
      if (!(frame instanceof HostRows)) return mergeProjectFrame(list, frame, LOCAL_HOST.id)
      return isConfigured(frame.hostId) ? withHostRows(list, frame.hostId, frame.rows) : list
    },
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
