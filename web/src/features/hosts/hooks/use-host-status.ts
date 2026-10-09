import { useCallback, useSyncExternalStore } from 'react'
import { getHostRuntime, subscribeHostStatus } from '@/features/hosts/stores/host-runtime-registry'
import { LOCAL_HOST, type HostStatus } from '@/features/hosts/types'
import { useConnectionStore, type ConnectionStatus } from '@/lib/ws/connection-store'

// The local daemon is not probed (a never-paired install sends nothing new): its
// streams already say whether it is reachable.
const LOCAL_STATUS: Record<ConnectionStatus, HostStatus> = {
  idle: 'unknown',
  connected: 'up',
  disconnected: 'down',
}

/** A host's status as the app last saw it — read only; the app's prober writes it. */
export function useHostStatus(hostId: string): HostStatus {
  const local = useConnectionStore((s) => LOCAL_STATUS[s.status])
  const subscribe = useCallback(
    (onChange: () => void) => subscribeHostStatus(hostId, onChange),
    [hostId],
  )
  const remote = useSyncExternalStore(
    subscribe,
    () => getHostRuntime(hostId)?.getState().status ?? 'unknown',
  )
  return hostId === LOCAL_HOST.id ? local : remote
}
