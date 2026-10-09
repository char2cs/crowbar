import { useCallback, useSyncExternalStore } from 'react'
import { hasPairedRemote, useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { LOCAL_HOST } from '@/features/hosts/types'
import { HostNotResolvedError } from '@/lib/host-path'
import { dataOf } from '@/lib/loadable'
import { useProjectDataStore } from '@/lib/store/projects'
import type { Project } from '@/lib/types'
import {
  getWorkspaceIdOwnedByChat,
  getWorkspaceScope,
  subscribeToWorkspaceScope,
} from '@/lib/workspace-scope'

type Resolution = { hostId: string } | { reason: string; projectId?: string }

const unresolved = (reason: string, projectId?: string): Resolution => ({ reason, projectId })

function heldProjects(): Project[] | undefined {
  return dataOf(useProjectDataStore.getState().data)
}

// The only proof that an unknown id is local: no remote was ever paired and no
// held row names one. Zero hosts alone is not — a revoked host's ids outlive its
// rows (scopes, route, panes), and a re-read without them must not read as local.
function noRemoteEntityCanExist(): boolean {
  if (hasPairedRemote(useHostConfigStore.getState())) return false
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
  // A twin on a configured-but-down host (a copied database) shows no second row.
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

// A chat has no row of its own here: it is placed through the workspace whose
// recorded scope names it as owner, and so through that workspace's project.
function resolveChatHost(chatId: string): Resolution {
  const wsId = getWorkspaceIdOwnedByChat(chatId)
  if (wsId === null) return absentEntity('no workspace scope records this chat as its owner')
  return resolveWorkspaceHost(wsId)
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

export function hostIdForChat(chatId: string): string {
  const resolved = resolveChatHost(chatId)
  if (!('hostId' in resolved)) {
    throw new HostNotResolvedError('chat', chatId, resolved.reason, resolved.projectId)
  }
  return resolved.hostId
}

/** The host `key` resolves to, or null while it does not — never `'local'` as a guess. */
export function resolvedHostOf(key: HostKey): string | null {
  const resolved =
    key.wsId !== undefined ? resolveWorkspaceHost(key.wsId) : resolveProjectHost(key.projectId)
  return 'hostId' in resolved ? resolved.hostId : null
}

export type HostKey = { projectId: string; wsId?: never } | { wsId: string; projectId?: never }

/** True when `hostIdForProject`/`hostIdForWorkspace` would not throw for `key`
 *  right now — the non-hook twin of `useHostResolved`, for stream engines. */
export function isHostResolved(key: HostKey): boolean {
  const resolved =
    key.wsId !== undefined ? resolveWorkspaceHost(key.wsId) : resolveProjectHost(key.projectId)
  return 'hostId' in resolved
}

/** Calls `onChange` whenever something `isHostResolved` reads may have changed:
 *  the host list, the project list and, for a workspace, its recorded scope. */
export function subscribeHostResolution(
  wsId: string | undefined,
  onChange: () => void,
): () => void {
  const stops = [useHostConfigStore.subscribe(onChange), useProjectDataStore.subscribe(onChange)]
  if (wsId !== undefined) stops.push(subscribeToWorkspaceScope(wsId, onChange))
  return () => stops.forEach((stop) => stop())
}

function useResolutionSubscription(key: HostKey) {
  const wsId = key.wsId
  return useCallback((onChange: () => void) => subscribeHostResolution(wsId, onChange), [wsId])
}

/** The host `key` resolves to, or null until it does — re-renders when the project
 *  list, the host list or the workspace's scope changes. Never `'local'` as a guess. */
export function useResolvedHost(key: HostKey): string | null {
  return useSyncExternalStore(useResolutionSubscription(key), () => resolvedHostOf(key))
}

/** True once `hostIdForProject`/`hostIdForWorkspace` would stop throwing for `key`. */
export function useHostResolved(key: HostKey): boolean {
  return useResolvedHost(key) !== null
}

/** Why a host that will not resolve is not coming: the loaded list does not hold its
 *  project, or holds it on a host that is not configured (or ambiguously). */
export type HostUnavailable = 'project-missing' | 'host-unavailable'

// Null while still waiting: the list has not loaded, or a workspace's scope (and so
// its project) is not recorded yet, however loaded the list.
function unavailability(key: HostKey): HostUnavailable | null {
  const { data } = useProjectDataStore.getState()
  if (data.status !== 'success' || isHostResolved(key)) return null
  const projectId = key.wsId !== undefined ? getWorkspaceScope(key.wsId)?.projectId : key.projectId
  if (projectId === undefined) return null
  return heldProjects()?.some((p) => p.id === projectId) ? 'host-unavailable' : 'project-missing'
}

/** `unavailability` as React state. */
export function useHostUnavailable(key: HostKey): HostUnavailable | null {
  return useSyncExternalStore(useResolutionSubscription(key), () => unavailability(key))
}

/** Settles with whether `key`'s host resolved: true as soon as it does, false once
 *  it is unavailable. No timer; it listens only until it has an answer. */
export function awaitHost(key: HostKey): Promise<boolean> {
  return new Promise((resolve) => {
    const settle = () => {
      const resolved = isHostResolved(key)
      if (!resolved && unavailability(key) === null) return
      stop()
      resolve(resolved)
    }
    const stop = subscribeHostResolution(key.wsId, settle)
    settle()
  })
}
