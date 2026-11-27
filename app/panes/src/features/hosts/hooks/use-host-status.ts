import { createContext, use, useCallback, useSyncExternalStore } from 'react'
import { hostStatusOf, subscribeHostStatusOf } from '@/features/hosts/lib/host-status'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { LOCAL_HOST, type HostDotState, type HostStatus } from '@/features/hosts/types'
import { useConnectionStore } from '@/lib/ws/connection-store'

function useHostStatus(hostId: string): HostStatus {
  const subscribe = useCallback(
    (onChange: () => void) => subscribeHostStatusOf(hostId, onChange),
    [hostId],
  )
  return useSyncExternalStore(subscribe, () => hostStatusOf(hostId))
}

export function useHostDotState(hostId: string): HostDotState {
  const status = useHostStatus(hostId)
  const streamsClosed = useConnectionStore((s) => s.hostStreams[hostId] === 'disconnected')
  return status === 'up' && streamsClosed ? 'no-live-stream' : status
}

interface HostView {
  name: string | undefined
  state: HostDotState
}

export const HostViewsContext = createContext<ReadonlyMap<string, HostView> | null>(null)

export function useHostView(hostId: string): HostView {
  const given = use(HostViewsContext)
  const name = useHostConfigStore((s) =>
    hostId === LOCAL_HOST.id ? LOCAL_HOST.name : s.hosts.find((h) => h.id === hostId)?.name,
  )
  const state = useHostDotState(hostId)
  if (given) return given.get(hostId) ?? { name: undefined, state: 'not-available' }
  return { name, state: name === undefined ? 'not-available' : state }
}
