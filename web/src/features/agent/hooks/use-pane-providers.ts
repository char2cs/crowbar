import { useEffect } from 'react'
import { useStore } from 'zustand'
import type { AgentProvider } from '@/features/agent/api/agent-api'
import { LOCAL_HOST } from '@/features/hosts/types'
import {
  providersOf,
  useAgentProvidersStore,
} from '@/features/settings/stores/agent-providers-store'
import type { WorkspaceStore } from '@/features/workspace/stores/workspace-store'

/**
 * The providers a chat pane offers: its workspace's own copy, else the list of the
 * daemon (`hostId`) that workspace lives on.
 *
 * `agentChats.providers` is seeded by use-workspace-agent-chats-stream, which only runs
 * while that workspace is MOUNTED, and a pane outlives that by design. Every provider
 * control is absence-not-disabled, so an empty catalogue DELETES the picker. With
 * neither copy populated the pane is the first surface to need the list, so it asks;
 * the store owns the read/write generation rules, so that load cannot clobber a
 * preferences write it races.
 */
export function usePaneProviders(store: WorkspaceStore, wsId: string, hostId: string) {
  const wsProviders = useStore(store, (s) => s.agentChats.providers)
  const daemonProviders = useAgentProvidersStore((s) => providersOf(s, hostId))
  useEffect(() => {
    if (wsProviders.length > 0 || daemonProviders.length > 0) return
    const { load, loadHost } = useAgentProvidersStore.getState()
    void (hostId === LOCAL_HOST.id ? load(wsId) : loadHost(hostId))
  }, [wsProviders.length, daemonProviders.length, wsId, hostId])
  const providers: AgentProvider[] = wsProviders.length > 0 ? wsProviders : daemonProviders
  return providers
}
