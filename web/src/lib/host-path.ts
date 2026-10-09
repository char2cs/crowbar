import { LOCAL_HOST } from '@/features/hosts/types'

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
