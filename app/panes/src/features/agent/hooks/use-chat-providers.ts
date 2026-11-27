import { useEffect } from 'react'
import { selectProjectsSynced, useSelect } from '@/core/entities/select'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'

export function useChatProviders(wsId: string): void {
  useEffect(() => {
    if (useAgentProvidersStore.getState().status === 'idle') {
      void useAgentProvidersStore.getState().load(wsId)
    }
  }, [wsId])
}

export function useChatListReady(): boolean {
  return useSelect(selectProjectsSynced)
}
