import { useEffect } from 'react'
import { selectProjectsSynced, useSelect } from '@/core/entities/select'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'

/** Loads the providers `wsId`'s chats run on, once per session. */
export function useChatProviders(wsId: string): void {
  useEffect(() => {
    if (useAgentProvidersStore.getState().status === 'idle') {
      void useAgentProvidersStore.getState().load(wsId)
    }
  }, [wsId])
}

/** Whether the chat list has landed: "not in the list" is final only then. */
export function useChatListReady(): boolean {
  return useSelect(selectProjectsSynced)
}
