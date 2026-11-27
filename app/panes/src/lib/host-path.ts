import { LOCAL_HOST } from '@/features/hosts/types'

const SCHEME = 'crowbar://'
const QUALIFIED = /^crowbar:\/\/([^/]*)([\s\S]*)$/
const SCHEME_LIKE = /^\s*crowbar:\/\//i
const DESKTOP_LOCAL_AUTHORITY = 'localhost'
const REMOTE_HOST_ID = /^[a-z0-9-]+$/

export interface HostPath {
  hostId: string
  path: string
}

export type HostUnavailable = 'project-missing' | 'host-unavailable'

export class HostNotResolvedError extends Error {
  constructor(
    readonly entity: 'project' | 'workspace' | 'chat',
    readonly id: string,
    readonly reason: string,
    readonly projectId?: string,
    readonly unavailable: HostUnavailable | null = null,
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

function assertRemotePath(path: string): void {
  if (!path.startsWith('/')) throw new Error(`host path must start with "/": "${path}"`)
  if (path.startsWith('//')) throw new Error(`host path must not start with "//": "${path}"`)
}

export function qualify(hostId: string, path: string): string {
  if (hostId === LOCAL_HOST.id) {
    if (SCHEME_LIKE.test(path)) throw new Error(`local host path must not be qualified: "${path}"`)
    return path
  }
  assertRemoteHostId(hostId)
  assertRemotePath(path)
  return `${SCHEME}${hostId}${path}`
}

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
