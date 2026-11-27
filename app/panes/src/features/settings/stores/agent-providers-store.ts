import { create } from 'zustand'
import { listGlobalProviders, listProviders } from '@/features/agent/api/agent-providers-api'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { onHostRemoved, useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { LOCAL_HOST } from '@/features/hosts/types'
import { resolvedHostOf } from '@/lib/host-resolve'

export type AgentProvidersStatus = 'idle' | 'loading' | 'ready' | 'failed'

interface AgentProvidersState {
  status: AgentProvidersStatus
  providers: AgentProvider[]
  remote: Record<string, AgentProvider[]>
  setProviders: (providers: AgentProvider[]) => void
  setHostProviders: (hostId: string, providers: AgentProvider[]) => void
  load: (wsId: string) => Promise<void>
  loadLocal: () => Promise<void>
  loadHost: (hostId: string) => Promise<AgentProvider[] | null>
  markUnavailable: () => void
}

let latestLoad = 0

let providerWrites = 0

export const beginProviderWrite = (): number => ++providerWrites

export const isLatestProviderWrite = (seq: number): boolean => seq === providerWrites

export const useAgentProvidersStore = create<AgentProvidersState>((set, get) => {
  const readLocal = async (fetchList: () => Promise<AgentProvider[]>) => {
    const seq = ++latestLoad
    const writes = providerWrites
    if (get().status !== 'ready') set({ status: 'loading' })
    try {
      const providers = await fetchList()
      if (seq !== latestLoad) return
      if (!isLatestProviderWrite(writes)) {
        set({ status: 'ready' })
        return
      }
      set({ providers, status: 'ready' })
    } catch {
      if (seq !== latestLoad) return
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

onHostRemoved((hostId) => {
  const { remote } = useAgentProvidersStore.getState()
  if (!(hostId in remote)) return
  useAgentProvidersStore.setState({
    remote: Object.fromEntries(Object.entries(remote).filter(([id]) => id !== hostId)),
  })
})

const NO_PROVIDERS: AgentProvider[] = []

export function providersOf(
  state: Pick<AgentProvidersState, 'providers' | 'remote'>,
  hostId: string | null,
): AgentProvider[] {
  if (hostId === null) return NO_PROVIDERS
  return hostId === LOCAL_HOST.id ? state.providers : (state.remote[hostId] ?? NO_PROVIDERS)
}
