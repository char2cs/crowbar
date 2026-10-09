import { LOCAL_HOST } from '@/features/hosts/types'

// A path is host-qualified as `crowbar://<hostId>/v0/...`; a bare `/v0/...` is
// the local daemon. The desktop proxy already spells its own local daemon
// `crowbar://localhost`, which must never be read as a remote host.
const SCHEME = 'crowbar://'
const QUALIFIED = /^crowbar:\/\/([^/]*)([\s\S]*)$/
// Any spelling of the scheme (case, leading space): never a plain local path.
const SCHEME_LIKE = /^\s*crowbar:\/\//i
const DESKTOP_LOCAL_AUTHORITY = 'localhost'
// What the desktop proxy accepts as a remote host id (hosts.rs). `localhost`
// and `local` both name the local daemon, so neither can be a remote id.
const REMOTE_HOST_ID = /^[a-z0-9-]+$/

export interface HostPath {
  hostId: string
  path: string
}

/** A host could not be determined for a project, workspace or chat; `reason` says
 *  why. Callers wait on `useHostResolved` or surface "still loading" — they
 *  never default to local. */
export class HostNotResolvedError extends Error {
  constructor(
    readonly entity: 'project' | 'workspace' | 'chat',
    readonly id: string,
    readonly reason: string,
    /** The project a workspace belongs to, when its scope names one. */
    readonly projectId?: string,
  ) {
    const owner = projectId === undefined ? '' : ` (project ${projectId})`
    super(`no host resolved for ${entity} ${id}${owner}: ${reason}`)
    this.name = 'HostNotResolvedError'
  }
}

function assertRemoteHostId(hostId: string): void {
  const reserved = hostId === DESKTOP_LOCAL_AUTHORITY || hostId === LOCAL_HOST.id
  if (reserved || !REMOTE_HOST_ID.test(hostId)) throw new Error(`invalid host id "${hostId}"`)
}

// A leading `//` would read as a different authority once the host is prefixed.
function assertRemotePath(path: string): void {
  if (!path.startsWith('/')) throw new Error(`host path must start with "/": "${path}"`)
  if (path.startsWith('//')) throw new Error(`host path must not start with "//": "${path}"`)
}

export function qualify(hostId: string, path: string): string {
  if (hostId === LOCAL_HOST.id) {
    // A local entity's call must not be able to land on another host.
    if (SCHEME_LIKE.test(path)) throw new Error(`local host path must not be qualified: "${path}"`)
    return path
  }
  assertRemoteHostId(hostId)
  assertRemotePath(path)
  return `${SCHEME}${hostId}${path}`
}

// Stricter than the proxy on purpose: `crowbar://localhost:<port>` and `crowbar:///`
// throw. `crowbar://editor/…` (Monaco URIs share the scheme) parses as host
// `editor` and fails later, at apiBaseFor.
export function splitHostPath(input: string): HostPath {
  if (SCHEME_LIKE.test(input) && !input.startsWith(SCHEME)) {
    throw new Error(`host path scheme must be exactly "${SCHEME}": "${input}"`)
  }
  const match = QUALIFIED.exec(input)
  if (!match) return { hostId: LOCAL_HOST.id, path: input }
  const [, authority, path] = match
  if (authority === DESKTOP_LOCAL_AUTHORITY) return { hostId: LOCAL_HOST.id, path }
  assertRemoteHostId(authority)
  assertRemotePath(path)
  return { hostId: authority, path }
}
