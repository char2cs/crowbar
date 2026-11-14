import { useEffect } from 'react'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { LOCAL_HOST } from '@/features/hosts/types'
import {
  providersOf,
  useAgentProvidersStore,
} from '@/features/settings/stores/agent-providers-store'

/** The providers a chat pane offers: the list of the daemon (`hostId`) its workspace
 *  lives on. An empty catalogue deletes every provider control, so a pane that is the
 *  first surface to need the list loads it. */
export function usePaneProviders(wsId: string, hostId: string): AgentProvider[] {
  const providers = useAgentProvidersStore((s) => providersOf(s, hostId))
  const empty = providers.length === 0
  useEffect(() => {
    const store = useAgentProvidersStore.getState()
    if (hostId !== LOCAL_HOST.id) {
      if (empty) void store.loadHost(hostId)
      return
    }
    if (store.status === 'idle') void store.load(wsId)
  }, [wsId, hostId, empty])
  return providers
}
