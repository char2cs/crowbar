import { createContext, use, useCallback, useSyncExternalStore } from 'react'
import { hostStatusOf, subscribeHostStatusOf } from '@/features/hosts/lib/host-status'
import { hostIsUnreachable } from '@/features/hosts/lib/host-tone'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { LOCAL_HOST, type HostDotState, type HostStatus } from '@/features/hosts/types'
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

interface HostView {
  name: string | undefined
  state: HostDotState
}

/** Host views handed in whole, for a page that renders a snapshot and watches no host itself
 *  (the phone's sidebar page). */
export const HostViewsContext = createContext<ReadonlyMap<string, HostView> | null>(null)

/** `hostId` as a surface names it: its name and dot. A host no longer configured has no name
 *  and is 'not-available' — never Local. */
export function useHostView(hostId: string): HostView {
  const given = use(HostViewsContext)
  const name = useHostConfigStore((s) =>
    hostId === LOCAL_HOST.id ? LOCAL_HOST.name : s.hosts.find((h) => h.id === hostId)?.name,
  )
  const state = useHostDotState(hostId)
  if (given) return given.get(hostId) ?? { name: undefined, state: 'not-available' }
  return { name, state: name === undefined ? 'not-available' : state }
}

/** Whether the sidebar greys out a project on `hostId`. Only a remote host is ever unreachable
 *  there: Local is the app itself, and a dropped local socket never greys a never-paired user's rows. */
export function useProjectHostUnreachable(hostId: string): boolean {
  const { state } = useHostView(hostId)
  return hostId !== LOCAL_HOST.id && hostIsUnreachable(state)
}
