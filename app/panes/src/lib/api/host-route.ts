import { splitHostPath, type HostPath } from '@/lib/host-path'

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
