import { useCallback, useSyncExternalStore } from 'react'
import { isHostResolved, subscribeHostResolution, type HostKey } from '@/lib/host-resolve'
import { dataOf } from '@/lib/loadable'
import { useProjectDataStore } from '@/lib/store/projects'
import { getWorkspaceScope } from '@/lib/workspace-scope'

/** Why a host that will not resolve is not coming: the loaded list does not hold its
 *  project, or holds it on a host that is not configured (or ambiguously). */
export type HostUnavailable = 'project-missing' | 'host-unavailable'

/** Why `key`'s host is not coming, or null while it is still worth waiting: the list
 *  has not loaded, or a workspace's scope (and so its project) is not recorded yet. */
export function hostUnavailability(key: HostKey): HostUnavailable | null {
  const { data } = useProjectDataStore.getState()
  if (data.status !== 'success' || isHostResolved(key)) return null
  const projectId = key.wsId !== undefined ? getWorkspaceScope(key.wsId)?.projectId : key.projectId
  if (projectId === undefined) return null
  return dataOf(data)?.some((p) => p.id === projectId) ? 'host-unavailable' : 'project-missing'
}

/** `hostUnavailability` as React state. */
export function useHostUnavailable(key: HostKey): HostUnavailable | null {
  const wsId = key.wsId
  const subscribe = useCallback(
    (onChange: () => void) => subscribeHostResolution(wsId, onChange),
    [wsId],
  )
  return useSyncExternalStore(subscribe, () => hostUnavailability(key))
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
