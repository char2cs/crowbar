import { followStreamFailures } from '@/features/hosts/lib/host-failures'
import { startHostProber } from '@/features/hosts/lib/host-prober'
import { getHostRuntime, subscribeHostStatus } from '@/features/hosts/stores/host-runtime-registry'
import { LOCAL_HOST, type HostStatus } from '@/features/hosts/types'
import { useConnectionStore, type ConnectionStatus } from '@/lib/ws/connection-store'

export function startHostStatus(): () => void {
  const stopProber = startHostProber()
  const stopFailures = followStreamFailures()
  return () => {
    stopProber()
    stopFailures()
  }
}

const LOCAL_STATUS: Record<ConnectionStatus, HostStatus> = {
  idle: 'unknown',
  connected: 'up',
  disconnected: 'down',
  migrating: 'down',
}

export function hostStatusOf(hostId: string): HostStatus {
  if (hostId === LOCAL_HOST.id) return LOCAL_STATUS[useConnectionStore.getState().status]
  return getHostRuntime(hostId)?.getState().status ?? 'unknown'
}

export function subscribeHostStatusOf(hostId: string, onChange: () => void): () => void {
  if (hostId === LOCAL_HOST.id) return useConnectionStore.subscribe(onChange)
  return subscribeHostStatus(hostId, onChange)
}

export function onHostUp(hostId: string, onUp: () => void): () => void {
  let was = hostStatusOf(hostId)
  return subscribeHostStatusOf(hostId, () => {
    const now = hostStatusOf(hostId)
    if (now === 'up' && was !== 'up') onUp()
    was = now
  })
}
