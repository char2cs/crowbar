import { useCallback, useSyncExternalStore } from 'react'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import {
  getHostRuntime,
  subscribeHostStatuses,
} from '@/features/hosts/stores/host-runtime-registry'
import { isHostResolved, subscribeHostResolution, type HostKey } from '@/lib/host-resolve'
import { dataOf } from '@/lib/loadable'
import { useProjectDataStore } from '@/lib/store/projects'
import { getWorkspaceScope } from '@/lib/workspace-scope'

/** Why a host that will not resolve is not coming: the loaded list does not hold its
 *  project, or holds it on a host that is not configured (or ambiguously). */
export type HostUnavailable = 'project-missing' | 'host-unavailable'

/** `key`'s project while a loaded list does not place its host; null while it resolves, the
 *  list has not loaded, or a workspace's scope (and so its project) is not recorded yet. */
function unplacedProject(key: HostKey): string | null {
  if (useProjectDataStore.getState().data.status !== 'success' || isHostResolved(key)) return null
  return (key.wsId !== undefined ? getWorkspaceScope(key.wsId)?.projectId : key.projectId) ?? null
}

/** Paired hosts not read since they last came up: a project in no row may be on any of them. */
function unreadHosts(): string[] {
  const { readHosts } = useProjectDataStore.getState()
  const hostIds = useHostConfigStore.getState().hosts.map((host) => host.id)
  return hostIds.filter((hostId) => !readHosts.has(hostId))
}

/** True once a loaded list holds no row of `projectId` and every paired host has said what it
 *  has: nothing more can bring the project back. */
export function isProjectGone(projectId: string): boolean {
  const { data } = useProjectDataStore.getState()
  if (data.status !== 'success' || dataOf(data)?.some((p) => p.id === projectId)) return false
  return unreadHosts().length === 0
}

/** Why `key`'s host is not coming, or null while it is still worth waiting: the list has not
 *  loaded, a workspace's scope is not recorded yet, or a paired host has not said what it has. */
export function hostUnavailability(key: HostKey): HostUnavailable | null {
  const projectId = unplacedProject(key)
  if (projectId === null) return null
  const held = dataOf(useProjectDataStore.getState().data)?.some((p) => p.id === projectId)
  if (held) return 'host-unavailable'
  return isProjectGone(projectId) ? 'project-missing' : null
}

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
