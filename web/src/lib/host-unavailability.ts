import { useCallback, useSyncExternalStore } from 'react'
import {
  getHostRuntime,
  subscribeHostStatuses,
} from '@/features/hosts/stores/host-runtime-registry'
import {
  hostUnavailability,
  isHostResolved,
  subscribeHostResolution,
  unplacedProject,
  unreadHosts,
  type HostKey,
} from '@/lib/host-resolve'
import type { HostUnavailable } from '@/lib/host-path'

// A wait only on hosts the app tried and could not reach is still a wait (one may come back
// with the project), but it is not "opening" any more: the pane says the host is not there.
function paneUnavailability(key: HostKey): HostUnavailable | null {
  const final = hostUnavailability(key)
  if (final !== null || unplacedProject(key) === null) return final
  const unreached = unreadHosts().every((hostId) => {
    const status = getHostRuntime(hostId)?.getState().status
    return status === 'down' || status === 'needs-repair'
  })
  return unreached ? 'host-unavailable' : null
}

/** What a pane waiting on `key`'s host says: why it is not coming (`hostUnavailability`),
 *  'host-unavailable' while it waits only on hosts that do not answer, or null (opening, or
 *  no key: nothing to wait on). */
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

/** Settles with whether `key`'s host resolved: true as soon as it does, false once
 *  it is unavailable. No timer; it listens only until it has an answer. */
export function awaitHost(key: HostKey): Promise<boolean> {
  return new Promise((resolve) => {
    const settle = () => {
      const resolved = isHostResolved(key)
      if (!resolved && hostUnavailability(key) === null) return
      stop()
      resolve(resolved)
    }
    const stop = subscribeHostResolution(key.wsId, settle)
    settle()
  })
}
