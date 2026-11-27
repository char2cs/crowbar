import { useCallback, useSyncExternalStore } from 'react'
import { hasPairedRemote, useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { LOCAL_HOST } from '@/features/hosts/types'
import { HostNotResolvedError, type HostUnavailable } from '@/lib/host-path'
import { entityStore, type EntityState } from '@/core/entities/entity-store'
import { entityKey, hostScope, splitHostKey } from '@/core/stream/host-scope'
import { hasLocalDaemon } from '@/features/hosts/lib/local-daemon'
import {
  getHostRuntime,
  subscribeHostStatuses,
} from '@/features/hosts/stores/host-runtime-registry'
import type { Project } from '@/lib/types'
import {
  getWorkspaceIdOwnedByChat,
  getWorkspaceScope,
  subscribeToWorkspaceScope,
} from '@/lib/workspace-scope'

type Resolution = { hostId: string } | { reason: string; projectId?: string }

const unresolved = (reason: string, projectId?: string): Resolution => ({ reason, projectId })

const listed = (s: EntityState, hostId: string) => !!s.loaded[hostScope(hostId, 'projects')]

export function unreadHosts(): string[] {
  const s = entityStore.getState()
  const unread: string[] = []
  for (const host of useHostConfigStore.getState().hosts) {
    if (!listed(s, host.id)) unread.push(host.id)
  }
  return unread
}

function listsLoaded(s: EntityState): boolean {
  if (hasLocalDaemon() && !listed(s, LOCAL_HOST.id)) return false
  const hosts = useHostConfigStore.getState().hosts.map((host) => host.id)
  const up = (hostId: string) => getHostRuntime(hostId)?.getState().status === 'up'
  if (hosts.some((hostId) => up(hostId) && !listed(s, hostId))) return false
  return hasLocalDaemon() || hosts.some((hostId) => listed(s, hostId))
}

function heldProjects(): Project[] {
  return Object.values(entityStore.getState().byKind.project).map((r) => r.data)
}

const loaded = () => listsLoaded(entityStore.getState())

type Placed = 'workspace' | 'chat'
const holders = new WeakMap<object, Map<string, string[]>>()

function hostsHolding(s: EntityState, kind: Placed, id: string): string[] {
  const records = s.byKind[kind]
  let byId = holders.get(records)
  if (!byId) {
    byId = new Map()
    for (const key of Object.keys(records)) {
      const { hostId, id: raw } = splitHostKey(key)
      byId.set(raw, [...(byId.get(raw) ?? []), hostId])
    }
    holders.set(records, byId)
  }
  return byId.get(id) ?? []
}

function noRemoteEntityCanExist(): boolean {
  if (hasPairedRemote(useHostConfigStore.getState())) return false
  return !heldProjects().some((p) => p.hostId !== LOCAL_HOST.id)
}

function absentEntity(reason: string, projectId?: string): Resolution {
  return noRemoteEntityCanExist() ? { hostId: LOCAL_HOST.id } : unresolved(reason, projectId)
}

function ownerOf(entity: string, hostIds: readonly string[], projectId?: string): Resolution {
  if (hostIds.length > 1) {
    return unresolved(
      `the ${entity} exists on more than one host (${hostIds.join(', ')})`,
      projectId,
    )
  }
  const [hostId] = hostIds
  const configured = useHostConfigStore.getState().hosts.some((h) => h.id === hostId)
  if (hostId === LOCAL_HOST.id || configured) return { hostId }
  return unresolved(`its host "${hostId}" is not configured`, projectId)
}

function resolveProjectHost(projectId: string): Resolution {
  const rows = heldProjects().filter((p) => p.id === projectId)
  if (rows.length === 0) {
    return absentEntity(
      loaded() ? 'the project is not in the project list' : 'the project list is not loaded',
    )
  }
  return ownerOf('project', [...new Set(rows.map((p) => p.hostId))])
}

function heldOwner(kind: Placed, id: string): Resolution | null {
  const s = entityStore.getState()
  const hostIds = hostsHolding(s, kind, id)
  if (hostIds.length === 0) return null
  const projectId = s.byKind[kind][entityKey(hostIds[0]!, id)]?.data.projectId
  return ownerOf(kind, hostIds, projectId)
}

function resolveWorkspaceHost(wsId: string): Resolution {
  const held = wsId ? heldOwner('workspace', wsId) : null
  if (held) return held
  const scope = wsId ? getWorkspaceScope(wsId) : null
  if (!scope) return absentEntity('no scope is recorded for the workspace')
  const project = resolveProjectHost(scope.projectId)
  return 'hostId' in project ? project : unresolved(project.reason, scope.projectId)
}

function resolveChatHost(chatId: string): Resolution {
  const held = heldOwner('chat', chatId)
  if (held) return held
  const wsId = getWorkspaceIdOwnedByChat(chatId)
  if (wsId === null) return absentEntity('no workspace scope records this chat as its owner')
  return resolveWorkspaceHost(wsId)
}

function notResolved(
  entity: HostNotResolvedError['entity'],
  id: string,
  { reason, projectId }: { reason: string; projectId?: string },
): HostNotResolvedError {
  const project = entity === 'project' ? id : projectId
  const unavailable = project === undefined ? null : hostUnavailability({ projectId: project })
  return new HostNotResolvedError(entity, id, reason, projectId, unavailable)
}

export function hostIdForProject(projectId: string): string {
  const resolved = resolveProjectHost(projectId)
  if (!('hostId' in resolved)) throw notResolved('project', projectId, resolved)
  return resolved.hostId
}

export function hostIdForWorkspace(wsId: string): string {
  const resolved = resolveWorkspaceHost(wsId)
  if (!('hostId' in resolved)) throw notResolved('workspace', wsId, resolved)
  return resolved.hostId
}

export function hostIdForChat(chatId: string): string {
  const resolved = resolveChatHost(chatId)
  if (!('hostId' in resolved)) throw notResolved('chat', chatId, resolved)
  return resolved.hostId
}

export function chatHostOf(chatId: string): string | null {
  const resolved = resolveChatHost(chatId)
  return 'hostId' in resolved ? resolved.hostId : null
}

const subscribeChatResolution = (onChange: () => void) =>
  subscribeHostResolution(undefined, onChange)

export function useChatHost(chatId: string): string | null {
  return useSyncExternalStore(subscribeChatResolution, () => chatHostOf(chatId))
}

export function resolvedHostOf(key: HostKey): string | null {
  const resolved =
    key.wsId !== undefined ? resolveWorkspaceHost(key.wsId) : resolveProjectHost(key.projectId)
  return 'hostId' in resolved ? resolved.hostId : null
}

export type HostKey = { projectId: string; wsId?: never } | { wsId: string; projectId?: never }

export function isHostResolved(key: HostKey): boolean {
  const resolved =
    key.wsId !== undefined ? resolveWorkspaceHost(key.wsId) : resolveProjectHost(key.projectId)
  return 'hostId' in resolved
}

export function subscribeHostResolution(
  wsId: string | undefined,
  onChange: () => void,
): () => void {
  const stops = [
    useHostConfigStore.subscribe(onChange),
    entityStore.subscribe(onChange),
    subscribeHostStatuses(onChange),
  ]
  if (wsId !== undefined) stops.push(subscribeToWorkspaceScope(wsId, onChange))
  return () => stops.forEach((stop) => stop())
}

function useResolutionSubscription(key: HostKey) {
  const wsId = key.wsId
  return useCallback((onChange: () => void) => subscribeHostResolution(wsId, onChange), [wsId])
}

export function useResolvedHost(key: HostKey): string | null {
  return useSyncExternalStore(useResolutionSubscription(key), () => resolvedHostOf(key))
}

export function useWorkspaceIsLocal(wsId: string): boolean {
  return useResolvedHost({ wsId }) === LOCAL_HOST.id
}

export function useHostResolved(key: HostKey): boolean {
  return useResolvedHost(key) !== null
}

export function unplacedProject(key: HostKey): string | null {
  if (!loaded() || isHostResolved(key)) return null
  return (key.wsId !== undefined ? getWorkspaceScope(key.wsId)?.projectId : key.projectId) ?? null
}

function heldProject(projectId: string): boolean {
  return heldProjects().some((p) => p.id === projectId)
}

export function isProjectGone(projectId: string): boolean {
  if (!loaded() || heldProject(projectId)) return false
  return unreadHosts().length === 0
}

export function hostUnavailability(key: HostKey): HostUnavailable | null {
  const projectId = unplacedProject(key)
  if (projectId === null) return null
  if (heldProject(projectId)) return 'host-unavailable'
  return isProjectGone(projectId) ? 'project-missing' : null
}
