import { useCallback, useSyncExternalStore } from 'react'
import { hostStatusOf, subscribeHostStatusOf } from '@/features/hosts/lib/host-status'
import type { HostDotState, HostStatus } from '@/features/hosts/types'
import { useConnectionStore } from '@/lib/ws/connection-store'

/** A host's status as the app last saw it — read only; its probes and streams keep it. */
function useHostStatus(hostId: string): HostStatus {
  const subscribe = useCallback(
    (onChange: () => void) => subscribeHostStatusOf(hostId, onChange),
    [hostId],
  )
  return useSyncExternalStore(subscribe, () => hostStatusOf(hostId))
}

/** What `hostId`'s status dot says. 'no-live-stream' while it answers but every live stream
 *  of it is closed (refused or dropped): what the app shows of it is not being kept current. */
export function useHostDotState(hostId: string): HostDotState {
  const status = useHostStatus(hostId)
  const streamsClosed = useConnectionStore((s) => s.hostStreams[hostId] === 'disconnected')
  return status === 'up' && streamsClosed ? 'no-live-stream' : status
}
