import { splitHostPath, type HostPath } from '@/lib/host-path'

// Imports only the pure host-path module: host-resolve imports the project
// store, which imports api.ts, so reaching it from here would be a cycle.

/**
 * The host a request is for and the daemon-relative path to send it. A path
 * may be host-qualified (`crowbar://<id>/v0/...`) and a caller may also name
 * the host; when both are present they must agree, because picking one
 * silently is how a request lands on the wrong daemon.
 */
export function routeToHost(path: string, hostId?: string): HostPath {
  const routed = splitHostPath(path)
  if (hostId === undefined) return routed
  if (routed.path !== path && routed.hostId !== hostId) {
    throw new Error(
      `request names two hosts: path is for "${routed.hostId}", hostId argument is "${hostId}"`,
    )
  }
  return { hostId, path: routed.path }
}
