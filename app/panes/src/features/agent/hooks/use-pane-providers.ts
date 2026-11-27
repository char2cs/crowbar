import { useEffect } from 'react'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { LOCAL_HOST } from '@/features/hosts/types'
import {
  providersOf,
  useAgentProvidersStore,
} from '@/features/settings/stores/agent-providers-store'

export function usePaneProviders(hostId: string): AgentProvider[] {
  const providers = useAgentProvidersStore((s) => providersOf(s, hostId))
  const empty = providers.length === 0
  useEffect(() => {
    const store = useAgentProvidersStore.getState()
    if (hostId !== LOCAL_HOST.id) {
      if (empty) void store.loadHost(hostId)
      return
    }
    if (store.status === 'idle') void store.loadLocal()
  }, [hostId, empty])
  return providers
}
