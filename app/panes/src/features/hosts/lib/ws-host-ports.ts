import type { WsHostPorts } from '@/lib/ws/ws-host-ports'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { getHostRuntime, subscribeHostStatus } from '@/features/hosts/stores/host-runtime-registry'
import { LOCAL_HOST } from '@/features/hosts/types'
import { hasLocalDaemon } from '@/features/hosts/lib/local-daemon'

function mayDial(hostId: string): boolean {
  if (hostId === LOCAL_HOST.id) return true
  if (!useHostConfigStore.getState().hosts.some((host) => host.id === hostId)) return false
  const status = getHostRuntime(hostId)?.getState().status
  return status !== 'down' && status !== 'needs-repair'
}

function onHostReady(hostId: string, listener: () => void): () => void {
  return subscribeHostStatus(hostId, () => {
    const isUp = getHostRuntime(hostId)?.getState().status === 'up'
    if (isUp && mayDial(hostId)) listener()
  })
}

/** The host layer as the socket manager asks for it; the local daemon is never probed, always dialled. */
export const hostPorts: WsHostPorts = {
  isLocalHost: (hostId) => hostId === LOCAL_HOST.id,
  mayDial,
  hasLocalDaemon,
  onHostReady,
}
