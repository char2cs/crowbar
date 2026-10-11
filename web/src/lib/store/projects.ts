import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { Project } from '@/lib/types'
import { dataOf, loading, success } from '@/lib/loadable'
import { loadCache } from '@/lib/persistence/cache-store'
import { createLoadableSlice, type LoadableSlice } from '@/lib/store/loadable-slice'
import { isReconnectSentinel } from '@/lib/ws/types'
import { onHostRemoved, useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { getHostRuntime, whileHostUp } from '@/features/hosts/stores/host-runtime-registry'
import { LOCAL_HOST } from '@/features/hosts/types'

/**
 * How the list reaches the daemons. Host resolution reads this store and the
 * transport builds its URLs through host resolution, so the transport is handed
 * in at boot (main.tsx) rather than imported: importing it was an import cycle.
 */
export interface ProjectListTransport {
  /** `hostId`'s projects, each tagged with that host. */
  read: (hostId: string) => Promise<Project[]>
  /** `hostId`'s `/v0/projects` frames; returns the unsubscribe. */
  subscribe: (hostId: string, onFrame: (frame: unknown) => void) => () => void
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
 * tombstone — folded into `hostId`'s rows. Every project write broadcasts every row
 * it changes, so the frames are the list and never need a re-read. A wire frame
 * names no host: it is the host of the socket it came on, whatever it says.
 */
function mergeProjectFrame(list: Project[], frame: unknown, hostId: string): Project[] | undefined {
  if (!frame || typeof frame !== 'object') return undefined
  const { status, ...fields } = frame as Partial<Project> & { id?: unknown; status?: string }
  if (typeof fields.id !== 'string') return undefined
  const project = { ...fields, hostId } as Project
  const index = list.findIndex((p) => p.id === project.id && p.hostId === hostId)
  if (status === 'deleted') return index === -1 ? list : list.filter((_, i) => i !== index)
  // A late frame from a host removed meanwhile: nothing could route the row it would add.
  if (hostId !== LOCAL_HOST.id && !isConfigured(hostId)) return list
  if (index !== -1 && JSON.stringify(list[index]) === JSON.stringify(project)) return list
  const next = index === -1 ? [...list, project] : list.map((p, i) => (i === index ? project : p))
  return next.sort(byOrder)
}

// Not 'projects': a list cached before projects carried a host would hand out rows
// with no hostId to route by.
const CACHE_KEY = 'projects-by-host'

/** One paired host's rows, read on their own: they replace that host's rows and no others. */
class HostRows {
  constructor(
    readonly hostId: string,
    readonly rows: Project[],
  ) {}
}

/** A frame from `hostId`'s own stream, or a project just created there: that host's row. */
class HostFrame {
  constructor(
    readonly hostId: string,
    readonly frame: unknown,
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

/** Per host, the frames each of its reads in flight has not seen: its answer may predate them. */
const unseenByReads = new Map<string, Set<unknown[]>>()

/**
 * Reads one paired host's projects and merges them in. A host that does not answer
 * keeps the rows it has: whether it is down is the prober's to say, not this read's.
 */
async function readHostRows(hostId: string): Promise<void> {
  const issued = (hostReads.get(hostId) ?? 0) + 1
  hostReads.set(hostId, issued)
  const unseen: unknown[] = []
  const reads = unseenByReads.get(hostId) ?? new Set<unknown[]>()
  reads.add(unseen)
  unseenByReads.set(hostId, reads)
  try {
    const rows = await connected()
      .read(hostId)
      .catch(() => null)
    if (rows === null || hostReads.get(hostId) !== issued) return
    const current = unseen.reduce<Project[]>(
      (list, frame) => mergeProjectFrame(list, frame, hostId) ?? list,
      rows,
    )
    await useProjectDataStore.getState().applyDelta(new HostRows(hostId, current))
  } finally {
    reads.delete(unseen)
    if (reads.size === 0) unseenByReads.delete(hostId)
  }
}

function onHostFrame(hostId: string, frame: unknown): void {
  // What the stream missed while it was down can only be read back, and only from that host.
  if (isReconnectSentinel(frame)) {
    void readHostRows(hostId)
    return
  }
  for (const unseen of unseenByReads.get(hostId) ?? []) unseen.push(frame)
  void useProjectDataStore.getState().applyDelta(new HostFrame(hostId, frame))
}

/**
 * A removed host's rows go the way a deleted project's do: a tombstone each, on whatever the
 * list holds — rows held while it is read, or after a read failed, are rows too.
 */
function dropHostRows(hostId: string): void {
  const { data, applyDelta } = useProjectDataStore.getState()
  const held = dataOf(data) ?? []
  const tombstones = held.flatMap((p) =>
    p.hostId === hostId ? [{ id: p.id, status: 'deleted' }] : [],
  )
  if (data.status === 'success') {
    for (const tombstone of tombstones) void applyDelta(new HostFrame(hostId, tombstone))
    return
  }
  // Not published: the next answer is rebuilt from what is held, so the tombstones go on that.
  if (tombstones.length === 0 || data.status === 'idle') return
  const staleData = tombstones.reduce(
    (list, tombstone) => mergeProjectFrame(list, tombstone, hostId) ?? list,
    held,
  )
  useProjectDataStore.setState({ data: { ...data, staleData } })
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
export const useProjectDataStore = create<LoadableSlice<Project[], []>>()((set, get) => {
  const slice = createLoadableSlice<Project[], []>({
    store: 'projects-data',
    // The local daemon's answer is published the moment it lands, with each paired host's
    // held rows; a host that is up merges in when it answers. Its reads start only after
    // a local success, so a failing local read never re-arms itself through them.
    fetcher: async () => {
      const local = await connected().read(LOCAL_HOST.id)
      for (const hostId of upHostIds()) void readHostRows(hostId)
      return withHeldRemoteRows(local, dataOf(get().data))
    },
    cacheKey: () => CACHE_KEY,
    subscribe: (onFrame) => connected().subscribe(LOCAL_HOST.id, onFrame),
    mergeFrame: (list, frame) => {
      if (frame instanceof HostFrame) return mergeProjectFrame(list, frame.frame, frame.hostId)
      if (!(frame instanceof HostRows)) return mergeProjectFrame(list, frame, LOCAL_HOST.id)
      return isConfigured(frame.hostId) ? withHostRows(list, frame.hostId, frame.rows) : list
    },
  })(set, get)
  return {
    ...slice,
    // Live: the local daemon's frames, and each paired host's own while it is up — read
    // afresh each time it comes up (at boot every host is unknown when the list is read).
    startSync: () => {
      const stopFrames = slice.startSync()
      const stopHosts = whileHostUp((hostId) => {
        void readHostRows(hostId)
        return connected().subscribe(hostId, (frame) => onHostFrame(hostId, frame))
      })
      const stopRemovals = onHostRemoved(dropHostRows)
      return () => {
        stopFrames()
        stopHosts()
        stopRemovals()
      }
    },
  }
})

/** Holds the cached list before its first read, so restored panes resolve their hosts at
 *  the first commit. The rows stay `loading` until that read replaces the local ones
 *  wholesale, so a project deleted meanwhile is gone at the first answer, not before. */
export async function seedProjectList(): Promise<void> {
  const cached = await loadCache<Project[]>('projects-data', CACHE_KEY)
  if (!cached || useProjectDataStore.getState().data.status !== 'idle') return
  useProjectDataStore.setState({ data: loading(success(cached.data, cached.fetchedAt)) })
}

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
  void useProjectDataStore.getState().applyDelta(new HostFrame(project.hostId, project))
}
