import { useCallback, useSyncExternalStore } from 'react'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { LOCAL_HOST } from '@/features/hosts/types'
import { HostNotResolvedError } from '@/lib/host-path'
import { dataOf } from '@/lib/loadable'
import { useProjectDataStore } from '@/lib/store/projects'
import type { Project } from '@/lib/types'
import { getWorkspaceScope, subscribeToWorkspaceScope } from '@/lib/workspace-scope'

type Resolution = { hostId: string } | { reason: string; projectId?: string }

const unresolved = (reason: string, projectId?: string): Resolution => ({ reason, projectId })

function heldProjects(): Project[] | undefined {
  return dataOf(useProjectDataStore.getState().data)
}

// The only proof that an entity we cannot find is local: no remote host is
// configured and no held row names one. Zero hosts alone is not proof — a
// revoked host's rows linger in the list and the cache.
function noRemoteEntityCanExist(): boolean {
  if (useHostConfigStore.getState().hosts.length > 0) return false
  return !heldProjects()?.some((p) => p.hostId !== LOCAL_HOST.id)
}

function absentEntity(reason: string, projectId?: string): Resolution {
  return noRemoteEntityCanExist() ? { hostId: LOCAL_HOST.id } : unresolved(reason, projectId)
}

// The entity's own row is looked up first; its host counts only while that host
// is local or still configured.
function resolveProjectHost(projectId: string): Resolution {
  const projects = heldProjects()
  const rows = projects?.filter((p) => p.id === projectId) ?? []
  if (rows.length === 0) {
    return absentEntity(
      projects ? 'the project is not in the project list' : 'the project list is not loaded',
    )
  }
  const hostIds = [...new Set(rows.map((p) => p.hostId))]
  if (hostIds.length > 1) {
    return unresolved(`the project exists on more than one host (${hostIds.join(', ')})`)
  }
  const [hostId] = hostIds
  const configured = useHostConfigStore.getState().hosts.some((h) => h.id === hostId)
  if (hostId === LOCAL_HOST.id || configured) return { hostId }
  return unresolved(`its host "${hostId}" is not configured`)
}

// The wsId is always passed: getWorkspaceScope(undefined) falls back to the
// ACTIVE workspace, which is how a call lands on the wrong host.
function resolveWorkspaceHost(wsId: string): Resolution {
  const scope = wsId ? getWorkspaceScope(wsId) : null
  if (!scope) return absentEntity('no scope is recorded for the workspace')
  const project = resolveProjectHost(scope.projectId)
  return 'hostId' in project ? project : unresolved(project.reason, scope.projectId)
}

export function hostIdForProject(projectId: string): string {
  const resolved = resolveProjectHost(projectId)
  if (!('hostId' in resolved)) throw new HostNotResolvedError('project', projectId, resolved.reason)
  return resolved.hostId
}

export function hostIdForWorkspace(wsId: string): string {
  const resolved = resolveWorkspaceHost(wsId)
  if (!('hostId' in resolved)) {
    throw new HostNotResolvedError('workspace', wsId, resolved.reason, resolved.projectId)
  }
  return resolved.hostId
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
  return useSyncExternalStore(subscribe, () => {
    const resolved =
      key.wsId !== undefined ? resolveWorkspaceHost(key.wsId) : resolveProjectHost(key.projectId)
    return 'hostId' in resolved
  })
}
