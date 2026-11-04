import { useCallback, useEffect, useState } from 'react'
import { updateProviderPreferences } from '@/features/agent/api/agent-providers-api'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { LOCAL_HOST } from '@/features/hosts/types'
import {
  applyProviderPreferences,
  buildProviderPreferences,
  providerDisabledMap,
} from '@/features/settings/components/tabs/provider-preferences'
import type { ProviderFlags } from '@/features/settings/components/tabs/provider-preferences'
import {
  beginProviderWrite,
  isLatestProviderWrite,
  providersOf,
  useAgentProvidersStore,
  type AgentProvidersStatus,
} from '@/features/settings/stores/agent-providers-store'
import { toast } from '@/features/window/stores/toast-store'

const publish = (hostId: string, resolved: AgentProvider[]) =>
  useAgentProvidersStore.getState().setHostProviders(hostId, resolved)

/** One daemon's list as Settings shows it, read on mount and on `retry`. The local daemon's
 *  state lives in the store; a remote host's load is tracked here, since the store keeps only its list. */
function useHostProviders(hostId: string): {
  providers: AgentProvider[]
  status: AgentProvidersStatus
  retry: () => void
} {
  const local = hostId === LOCAL_HOST.id
  const providers = useAgentProvidersStore((s) => providersOf(s, hostId))
  const localStatus = useAgentProvidersStore((s) => s.status)
  const [remoteStatus, setRemoteStatus] = useState<AgentProvidersStatus>('loading')
  const [attempt, setAttempt] = useState(0)

  useEffect(() => {
    if (local) {
      void useAgentProvidersStore.getState().loadLocal()
      return
    }
    let current = true
    setRemoteStatus('loading')
    void useAgentProvidersStore
      .getState()
      .loadHost(hostId)
      .then((resolved) => {
        if (current) setRemoteStatus(resolved ? 'ready' : 'failed')
      })
    return () => {
      current = false
    }
  }, [hostId, local, attempt])

  const retry = useCallback(() => setAttempt((n) => n + 1), [])
  return { providers, status: local ? localStatus : remoteStatus, retry }
}

/**
 * The Agents tab's state and its one write path, for the daemon of `hostId`. Every change —
 * a reorder, an enable toggle, a tools toggle — is the same pair (the new order, the complete
 * flags table): the PUT replaces whole rows, so a write that knew only one flag would zero
 * the other.
 */
export function useProvidersSettings(hostId: string = LOCAL_HOST.id) {
  const { providers, status, retry } = useHostProviders(hostId)

  const commit = useCallback(
    async (orderedIds: string[], flagsById: Record<string, ProviderFlags>) => {
      const prefs = buildProviderPreferences(orderedIds, flagsById)
      const previous = providersOf(useAgentProvidersStore.getState(), hostId)

      // OPTIMISTIC, under the store's write generation, which also fences the reads: a GET
      // issued before this write and answered after it must not reinstate the old list.
      const seq = beginProviderWrite()
      publish(hostId, applyProviderPreferences(previous, orderedIds, flagsById))
      try {
        const resolved = await (hostId === LOCAL_HOST.id
          ? updateProviderPreferences(prefs)
          : updateProviderPreferences(prefs, hostId))
        if (!isLatestProviderWrite(seq)) return // overtaken — a newer write owns the store
        publish(hostId, resolved)
      } catch {
        // A superseded write neither rolls back nor complains: the newer payload carries it.
        if (!isLatestProviderWrite(seq)) return
        publish(hostId, previous)
        toast.error('Could not save providers', 'Crowbar could not reach the daemon — try again.')
      }
    },
    [hostId],
  )

  const toggle = useCallback(
    (id: string, enabled: boolean) => {
      const flagsById = providerDisabledMap(providers)
      // Flip ONE FIELD of one row; every other flag is carried through as the list has it.
      void commit(
        providers.map((p) => p.id),
        {
          ...flagsById,
          [id]: { disabled: !enabled, mcpDisabled: flagsById[id]?.mcpDisabled ?? false },
        },
      )
    },
    [providers, commit],
  )

  const toggleTools = useCallback(
    (id: string, enabled: boolean) => {
      const flagsById = providerDisabledMap(providers)
      void commit(
        providers.map((p) => p.id),
        {
          ...flagsById,
          [id]: { disabled: flagsById[id]?.disabled ?? false, mcpDisabled: !enabled },
        },
      )
    },
    [providers, commit],
  )

  const reorder = useCallback(
    (orderedIds: string[]) => void commit(orderedIds, providerDisabledMap(providers)),
    [providers, commit],
  )

  return { providers, status, retry, toggle, toggleTools, reorder }
}
