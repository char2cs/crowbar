import { useCallback, useSyncExternalStore } from 'react'
import {
  getHostRuntime,
  subscribeHostStatuses,
} from '@/features/hosts/stores/host-runtime-registry'
import {
  hostUnavailability,
  subscribeHostResolution,
  unplacedProject,
  unreadHosts,
  type HostKey,
} from '@/lib/host-resolve'
import type { HostUnavailable } from '@/lib/host-path'

function paneUnavailability(key: HostKey): HostUnavailable | null {
  const final = hostUnavailability(key)
  if (final !== null || unplacedProject(key) === null) return final
  const unreached = unreadHosts().every((hostId) => {
    const status = getHostRuntime(hostId)?.getState().status
    return status === 'down' || status === 'needs-repair'
  })
  return unreached ? 'host-unavailable' : null
}

export function useHostUnavailable(key: HostKey | null): HostUnavailable | null {
  const wsId = key?.wsId
  const subscribe = useCallback(
    (onChange: () => void) => {
      const stops = [subscribeHostResolution(wsId, onChange), subscribeHostStatuses(onChange)]
      return () => stops.forEach((stop) => stop())
    },
    [wsId],
  )
  return useSyncExternalStore(subscribe, () => (key === null ? null : paneUnavailability(key)))
}
