import { useCallback, useSyncExternalStore } from 'react'
import { hostStatusOf, subscribeHostStatusOf } from '@/features/hosts/lib/host-status'
import type { HostStatus } from '@/features/hosts/types'
import { useConnectionStore } from '@/lib/ws/connection-store'

/** A host's status as the app last saw it — read only; its probes and streams keep it. */
export function useHostStatus(hostId: string): HostStatus {
  const subscribe = useCallback(
    (onChange: () => void) => subscribeHostStatusOf(hostId, onChange),
    [hostId],
  )
  return useSyncExternalStore(subscribe, () => hostStatusOf(hostId))
}

/** True while every live stream of remote `hostId` is closed (refused or dropped): what the
 *  app shows of that host is not being kept current, however its status reads. */
export function useHostStreamsClosed(hostId: string): boolean {
  return useConnectionStore((s) => s.hostStreams[hostId] === 'disconnected')
}
