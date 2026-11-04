import { assetURL } from '@/lib/api'
import { qualify } from '@/lib/host-path'
import { hostIdForProject } from '@/lib/host-resolve'

// Builders hand `qualify` BARE `/v0/...` paths (desktop's API_BASE is itself `crowbar://localhost`).
// The host is the one the project's own row names, so no call site passes it; an unplaceable
// project throws HostNotResolvedError rather than reading as local.

function projectPath(projectId: string): string {
  return `/v0/projects/${encodeURIComponent(projectId)}`
}

/** The REST/WS base of a project: `/v0/projects/:p`, qualified with the project's host. */
export function projectBase(projectId: string): string {
  return qualify(hostIdForProject(projectId), projectPath(projectId))
}

/** {@link projectBase} for a host the caller chose itself — the project it just created there
 *  is not in the project list yet, so its row cannot name the host. */
export function projectBaseOnHost(hostId: string, projectId: string): string {
  return qualify(hostId, projectPath(projectId))
}

/** The REST/WS base of a repo: `/v0/projects/:p/repos/:r`, qualified with its project's host. */
export function repoBase(projectId: string, repoId: string): string {
  const path = `${projectPath(projectId)}/repos/${encodeURIComponent(repoId)}`
  return qualify(hostIdForProject(projectId), path)
}

/**
 * A URL the webview can load for a project's or repo's icon: the DTO's bare `/v0/...` path,
 * resolved against the project's own host, never the local daemon.
 */
export function projectAssetURL(projectId: string, path: string): string {
  return assetURL(qualify(hostIdForProject(projectId), path))
}
