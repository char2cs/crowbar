import { create } from 'zustand'
import { listGlobalProviders, listProviders } from '@/features/agent/api/agent-api'
import type { AgentProvider } from '@/features/agent/api/agent-api'
import { onHostRemoved, useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { LOCAL_HOST } from '@/features/hosts/types'
import { resolvedHostOf } from '@/lib/host-resolve'

/**
 * What the app knows about the agent providers RIGHT NOW — kept apart from
 * `providers` and `[]` because those two answers together cannot say anything
 * useful. `ready` + empty is the daemon reporting none; `failed` is Crowbar not
 * having been able to ask. Rendering "No providers available." for both was
 * actively misleading — the live report that produced this store was a user
 * reading that sentence with both providers installed and enabled on disk.
 */
export type AgentProvidersStatus = 'idle' | 'loading' | 'ready' | 'failed'

interface AgentProvidersState {
  /** The LOCAL daemon's list and its state — what Settings shows and edits. */
  status: AgentProvidersStatus
  providers: AgentProvider[]
  /** Remote daemons' lists by host id, absent until read. A provider list belongs
   *  to the daemon that reports it; one host's must never stand in for another's. */
  remote: Record<string, AgentProvider[]>
  /**
   * Adopt a list resolved somewhere else — a workspace's own seed, or the
   * response to a preferences PUT (which returns the freshly resolved set).
   */
  setProviders: (providers: AgentProvider[]) => void
  /** `setProviders` for the daemon of `hostId`: the local list for 'local', else its own entry. */
  setHostProviders: (hostId: string, providers: AgentProvider[]) => void
  /**
   * Fetch the local list through `wsId` for workspace chat surfaces. A workspace
   * on a remote host is not the local daemon's to describe: nothing is asked and
   * null comes back (see loadHost).
   *
   * Returns the resolved list so the caller can mirror it into the workspace
   * store the chat surfaces read, or null when this load did not win.
   */
  load: (wsId: string) => Promise<AgentProvider[] | null>
  /** `load` through the global route, which needs no workspace in view (Settings). */
  loadLocal: () => Promise<AgentProvider[] | null>
  /** Read a remote host's own list into `remote[hostId]`; null when it did not answer. */
  loadHost: (hostId: string) => Promise<AgentProvider[] | null>
  /** The daemon could not answer. Say we could not tell — never that there are none. */
  markUnavailable: () => void
}

// Only the most-recently ISSUED load may write, exactly as loadable-slice's
// `latestFetch` does: resolution order is not issue order, and an older answer
// landing last would reinstate a list the user has already moved past.
let latestLoad = 0

/**
 * THE WRITE GENERATION. Bumped once per preference WRITE, at the moment the
 * write is issued, and read by everything that publishes a provider list.
 *
 * It exists because sequencing writes against writes is only half the problem.
 * A GET is a snapshot of the server BEFORE the PUT it overlaps, so a read issued
 * first and answered second re-installs the pre-write list — the switch the user
 * just moved visibly flips back, and the next write PUTs that stale flag. The
 * mount refetch and the workspace-stream reseed are both such reads.
 *
 * A READ therefore never bumps this; it captures the value it started under and
 * publishes only if nothing has been written since. That direction is the whole
 * design: a read that bumped would invalidate the in-flight write it raced and
 * then install its own stale answer, which is the same bug with the roles
 * swapped.
 */
let providerWrites = 0

/** Claim a generation for a write about to be issued. */
export const beginProviderWrite = (): number => ++providerWrites

/** The generation a read is starting under; pair with isLatestProviderWrite. */
export const providerWriteGeneration = (): number => providerWrites

/**
 * Whether `seq` is still current — for a write, that it has not been superseded;
 * for a read, that no write has been issued since it asked.
 */
export const isLatestProviderWrite = (seq: number): boolean => seq === providerWrites

/**
 * The provider lists the app holds outside any one workspace store. A provider
 * list belongs to a daemon, so the top-level fields are the LOCAL daemon's (the
 * Settings dialog edits exactly that) and `remote` holds each other host's own.
 *
 * The per-workspace copy still feeds the chat surfaces; the local list survives
 * having no workspace in view. Both are written from the same two events — a
 * workspace seed and a preferences PUT — so they cannot disagree.
 */
export const useAgentProvidersStore = create<AgentProvidersState>((set, get) => {
  // One read of the LOCAL daemon's list, whichever route reaches it.
  const readLocal = async (fetchList: () => Promise<AgentProvider[]>) => {
    const seq = ++latestLoad
    // The write generation this read is a snapshot of. Anything written after
    // this line describes a server this answer has not seen.
    const writes = providerWriteGeneration()
    // Only a load with no settled answer behind it may show a spinner. `ready`
    // is settled even when the list is EMPTY — "the daemon has none" is an
    // answer — so a refresh keeps showing what it says instead of strobing
    // between a spinner and a sentence. A retry after `failed` does show it.
    if (get().status !== 'ready') set({ status: 'loading' })
    try {
      const providers = await fetchList()
      if (seq !== latestLoad) return null
      // A write landed while this was in flight. Its own optimistic state — and
      // the response it reconciles from — are newer than this snapshot, so
      // publishing here would undo it in front of the user: the switch they just
      // moved flips back, and the next write PUTs the flag this list carries.
      // The status still settles: we did reach the daemon, and leaving it on
      // `loading` would strand the tab's spinner.
      if (!isLatestProviderWrite(writes)) {
        set({ status: 'ready' })
        return null
      }
      set({ providers, status: 'ready' })
      return providers
    } catch {
      if (seq !== latestLoad) return null
      // A failed REFRESH must not empty a good list — the data we hold is still
      // the last thing the server said.
      set({ status: get().providers.length > 0 ? 'ready' : 'failed' })
      return null
    }
  }

  return {
    status: 'idle',
    providers: [],
    remote: {},

    setProviders: (providers) => set({ providers, status: 'ready' }),

    setHostProviders: (hostId, providers) => {
      if (hostId === LOCAL_HOST.id) get().setProviders(providers)
      // A late answer from a host removed meanwhile is nobody's list any more.
      else if (useHostConfigStore.getState().hosts.some((host) => host.id === hostId)) {
        set((s) => ({ remote: { ...s.remote, [hostId]: providers } }))
      }
    },

    load: async (wsId) =>
      resolvedHostOf({ wsId }) === LOCAL_HOST.id ? readLocal(() => listProviders(wsId)) : null,

    loadLocal: () => readLocal(() => listGlobalProviders()),

    loadHost: async (hostId) => {
      try {
        const providers = await listGlobalProviders(hostId)
        get().setHostProviders(hostId, providers)
        return providers
      } catch {
        return null
      }
    },

    markUnavailable: () => {
      if (get().providers.length > 0) return
      set({ status: 'failed' })
    },
  }
})

// A removed host's list goes with it, whichever window removed it.
onHostRemoved((hostId) => {
  const { remote } = useAgentProvidersStore.getState()
  if (!(hostId in remote)) return
  useAgentProvidersStore.setState({
    remote: Object.fromEntries(Object.entries(remote).filter(([id]) => id !== hostId)),
  })
})

const NO_PROVIDERS: AgentProvider[] = []

/** The providers of `hostId`'s daemon as far as they are known (stable when empty);
 *  nothing for a null host, which is not yet known. */
export function providersOf(
  state: Pick<AgentProvidersState, 'providers' | 'remote'>,
  hostId: string | null,
): AgentProvider[] {
  if (hostId === null) return NO_PROVIDERS
  return hostId === LOCAL_HOST.id ? state.providers : (state.remote[hostId] ?? NO_PROVIDERS)
}
