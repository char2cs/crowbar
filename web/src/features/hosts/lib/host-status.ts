import { followStreamFailures } from '@/features/hosts/lib/host-failures'
import { startHostProber } from '@/features/hosts/lib/host-prober'
import { getHostRuntime, subscribeHostStatus } from '@/features/hosts/stores/host-runtime-registry'
import { LOCAL_HOST, type HostStatus } from '@/features/hosts/types'
import { useConnectionStore, type ConnectionStatus } from '@/lib/ws/connection-store'

/** Keeps every paired host's status for the app's life: its probes, and between two probes
 *  whatever its streams are refused with. */
export function startHostStatus(): () => void {
  const stopProber = startHostProber()
  const stopFailures = followStreamFailures()
  return () => {
    stopProber()
    stopFailures()
  }
}

// The local daemon is not probed (a never-paired install sends nothing new): its own
// streams are what say it is reachable.
const LOCAL_STATUS: Record<ConnectionStatus, HostStatus> = {
  idle: 'unknown',
  connected: 'up',
  disconnected: 'down',
}

/** Whether `hostId` is reachable, as the app knows it: the one answer every caller reads. */
export function hostStatusOf(hostId: string): HostStatus {
  if (hostId === LOCAL_HOST.id) return LOCAL_STATUS[useConnectionStore.getState().status]
  return getHostRuntime(hostId)?.getState().status ?? 'unknown'
}

/** Calls `onChange` whenever `hostStatusOf(hostId)` may have changed. */
export function subscribeHostStatusOf(hostId: string, onChange: () => void): () => void {
  if (hostId === LOCAL_HOST.id) return useConnectionStore.subscribe(onChange)
  return subscribeHostStatus(hostId, onChange)
}

/** Calls `onUp` each time `hostId` becomes 'up' from anything else. */
export function onHostUp(hostId: string, onUp: () => void): () => void {
  let was = hostStatusOf(hostId)
  return subscribeHostStatusOf(hostId, () => {
    const now = hostStatusOf(hostId)
    if (now === 'up' && was !== 'up') onUp()
    was = now
  })
}
