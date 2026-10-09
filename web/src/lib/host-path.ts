import { useCallback, useSyncExternalStore } from 'react'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { LOCAL_HOST } from '@/features/hosts/types'
import { dataOf } from '@/lib/loadable'
import { useProjectDataStore } from '@/lib/store/projects'
import { getWorkspaceScope, subscribeToWorkspaceScope } from '@/lib/workspace-scope'

// A path is host-qualified as `crowbar://<hostId>/v0/...`; a bare `/v0/...` is
// the local daemon. The desktop proxy already spells its own local daemon
// `crowbar://localhost`, which must never be read as a remote host.
const QUALIFIED = /^crowbar:\/\/([^/]*)([\s\S]*)$/
const DESKTOP_LOCAL_AUTHORITY = 'localhost'
// What the desktop proxy accepts as a remote host id (hosts.rs). `localhost`
// and `local` both name the local daemon, so neither can be a remote id.
const REMOTE_HOST_ID = /^[a-z0-9-]+$/

export interface HostPath {
  hostId: string
  path: string
}

/** A host could not be determined for a project or workspace. Callers wait on
 *  `useHostResolved` or surface "still loading" — they never default to local. */
export class HostNotResolvedError extends Error {
  constructor(
    readonly entity: 'project' | 'workspace',
    readonly id: string,
  ) {
    super(`no host resolved for ${entity} ${id}`)
    this.name = 'HostNotResolvedError'
  }
}

function assertRemoteHostId(hostId: string): void {
  const reserved = hostId === DESKTOP_LOCAL_AUTHORITY || hostId === LOCAL_HOST.id
  if (reserved || !REMOTE_HOST_ID.test(hostId)) throw new Error(`invalid host id "${hostId}"`)
}

function assertRemotePath(path: string): void {
  if (!path.startsWith('/')) throw new Error(`host path must start with "/": "${path}"`)
}

export function qualify(hostId: string, path: string): string {
  if (hostId === LOCAL_HOST.id) return path
  assertRemoteHostId(hostId)
  assertRemotePath(path)
  return `crowbar://${hostId}${path}`
}

export function splitHostPath(input: string): HostPath {
  const match = QUALIFIED.exec(input)
  if (!match) return { hostId: LOCAL_HOST.id, path: input }
  const [, authority, path] = match
  if (authority === DESKTOP_LOCAL_AUTHORITY) return { hostId: LOCAL_HOST.id, path }
  assertRemoteHostId(authority)
  assertRemotePath(path)
  return { hostId: authority, path }
}

// With no remote host configured no remote entity can exist, so local is true
// by construction; with one, local must be read off the entity, never assumed.
function hasRemoteHosts(): boolean {
  return useHostConfigStore.getState().hosts.length > 0
}

function resolveProjectHost(projectId: string): string | null {
  if (!hasRemoteHosts()) return LOCAL_HOST.id
  const projects = dataOf(useProjectDataStore.getState().data)
  return projects?.find((p) => p.id === projectId)?.hostId ?? null
}

// The wsId is always passed: getWorkspaceScope(undefined) falls back to the
// ACTIVE workspace, which is how a call lands on the wrong host.
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
