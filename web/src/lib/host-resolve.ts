import { useCallback, useSyncExternalStore } from 'react'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { LOCAL_HOST } from '@/features/hosts/types'
import { HostNotResolvedError } from '@/lib/host-path'
import { dataOf } from '@/lib/loadable'
import { useProjectDataStore } from '@/lib/store/projects'
import { getWorkspaceScope, subscribeToWorkspaceScope } from '@/lib/workspace-scope'

// With no remote host configured no remote entity can exist, so local is true
// by construction (and local-only behaviour stays unchanged); with one, local
// must be read off the entity, never assumed.
function hasRemoteHosts(): boolean {
  return useHostConfigStore.getState().hosts.length > 0
}

function resolveProjectHost(projectId: string): string | null {
  if (!hasRemoteHosts()) return LOCAL_HOST.id
  const projects = dataOf(useProjectDataStore.getState().data)
  return projects?.find((p) => p.id === projectId)?.hostId ?? null
}

// Same zero-hosts shortcut as projects, so it precedes the scope lookup. The
// wsId is always passed: getWorkspaceScope(undefined) falls back to the ACTIVE
// workspace, which is how a call lands on the wrong host.
function resolveWorkspaceHost(wsId: string): string | null {
  if (!hasRemoteHosts()) return LOCAL_HOST.id
  if (!wsId) return null
  const scope = getWorkspaceScope(wsId)
  return scope ? resolveProjectHost(scope.projectId) : null
}

export function hostIdForProject(projectId: string): string {
  const hostId = resolveProjectHost(projectId)
  if (hostId === null) throw new HostNotResolvedError('project', projectId)
  return hostId
}

export function hostIdForWorkspace(wsId: string): string {
  const hostId = resolveWorkspaceHost(wsId)
  if (hostId === null) throw new HostNotResolvedError('workspace', wsId)
  return hostId
}

export type HostKey = { projectId: string; wsId?: never } | { wsId: string; projectId?: never }

/** True once `hostIdForProject`/`hostIdForWorkspace` would stop throwing for
 *  `key` — re-renders when the project list, the host list or the workspace's
 *  scope changes. */
export function useHostResolved(key: HostKey): boolean {
  const wsId = key.wsId
  const subscribe = useCallback(
    (onChange: () => void) => {
      const stops = [
        useHostConfigStore.subscribe(onChange),
        useProjectDataStore.subscribe(onChange),
      ]
      if (wsId !== undefined) stops.push(subscribeToWorkspaceScope(wsId, onChange))
      return () => stops.forEach((stop) => stop())
    },
    [wsId],
  )
  return useSyncExternalStore(
    subscribe,
    () =>
      (key.wsId !== undefined
        ? resolveWorkspaceHost(key.wsId)
        : resolveProjectHost(key.projectId)) !== null,
  )
}
