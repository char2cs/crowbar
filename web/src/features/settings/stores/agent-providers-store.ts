import { create } from 'zustand'
import { listGlobalProviders, listProviders } from '@/features/agent/api/agent-providers-api'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { onHostRemoved, useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { LOCAL_HOST } from '@/features/hosts/types'
import { resolvedHostOf } from '@/lib/host-resolve'

// Separate from `providers`: `ready` + empty is the daemon reporting none, `failed` is Crowbar
// unable to ask, and the UI must not say "No providers" for both.
export type AgentProvidersStatus = 'idle' | 'loading' | 'ready' | 'failed'

interface AgentProvidersState {
  /** The LOCAL daemon's list and its state — what Settings shows and edits. */
  status: AgentProvidersStatus
  providers: AgentProvider[]
  /** Remote daemons' lists by host id, absent until read. A provider list belongs
   *  to the daemon that reports it; one host's must never stand in for another's. */
  remote: Record<string, AgentProvider[]>
  /** Adopt a list resolved elsewhere: a preferences PUT answers the fresh set. */
  setProviders: (providers: AgentProvider[]) => void
  /** `setProviders` for the daemon of `hostId`: the local list for 'local', else its own entry. */
  setHostProviders: (hostId: string, providers: AgentProvider[]) => void
  /** Fetch the list of `wsId`'s daemon: the local one through the workspace, a remote
   *  host's through loadHost; nothing while the workspace's host is unknown. */
  load: (wsId: string) => Promise<void>
  /** The local list through the global route, which needs no workspace in view (Settings). */
  loadLocal: () => Promise<void>
  /** Read a remote host's own list into `remote[hostId]`; null when it did not answer. */
  loadHost: (hostId: string) => Promise<AgentProvider[] | null>
  /** The daemon could not answer. Say we could not tell — never that there are none. */
  markUnavailable: () => void
}

// Only the most-recently ISSUED load may write: resolution order is not issue order, and an older answer
// landing last would reinstate a list the user has already moved past.
let latestLoad = 0

// Write generation, bumped when a preference write is issued. A read overlapping a PUT snapshots
// the pre-write list, so reads capture it and publish only if unchanged; a read must never bump it
// (it would invalidate the write it raced and install its own stale answer).
let providerWrites = 0

/** Claim a generation for a write about to be issued. */
export const beginProviderWrite = (): number => ++providerWrites

/** For a write: not superseded. For a read: no write issued since it asked. */
export const isLatestProviderWrite = (seq: number): boolean => seq === providerWrites

/** The provider lists of the daemons the app talks to: the top-level fields are the
 *  LOCAL daemon's (Settings edits exactly that) and `remote` holds each other host's. */
export const useAgentProvidersStore = create<AgentProvidersState>((set, get) => {
  // One read of the LOCAL daemon's list, whichever route reaches it.
  const readLocal = async (fetchList: () => Promise<AgentProvider[]>) => {
    const seq = ++latestLoad
    // The write generation this read is a snapshot of. Anything written after
    // this line describes a server this answer has not seen.
    const writes = providerWrites
    // Only an unsettled load shows a spinner; `ready` (even empty) is an answer, so a refresh
    // doesn't strobe. A retry after `failed` does show it.
    if (get().status !== 'ready') set({ status: 'loading' })
    try {
      const providers = await fetchList()
      if (seq !== latestLoad) return
      // A write was issued meanwhile and is newer than this snapshot, so don't publish; still
      // settle the status (the daemon answered) so the spinner isn't stranded.
      if (!isLatestProviderWrite(writes)) {
        set({ status: 'ready' })
        return
      }
      set({ providers, status: 'ready' })
    } catch {
      if (seq !== latestLoad) return
      // A failed REFRESH must not empty a good list — the data we hold is still
      // the last thing the server said.
      set({ status: get().providers.length > 0 ? 'ready' : 'failed' })
    }
  }

  const loadHost = async (hostId: string) => {
    try {
      const providers = await listGlobalProviders(hostId)
      get().setHostProviders(hostId, providers)
      return providers
    } catch {
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

    load: async (wsId) => {
      const hostId = resolvedHostOf({ wsId })
      if (hostId === LOCAL_HOST.id) await readLocal(() => listProviders(wsId))
      else if (hostId !== null) await loadHost(hostId)
    },

    loadLocal: () => readLocal(() => listGlobalProviders()),

    loadHost,

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
