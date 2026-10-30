import { hasLocalDaemon } from '@/features/hosts/lib/local-daemon'
import { LOCAL_HOST, type Host } from '@/features/hosts/types'
import type { Project } from '@/lib/types'

/** Every host the sidebar can filter to: this machine's daemon, if it has one, then each paired host. */
export function sidebarHostIds(paired: readonly Host[]): string[] {
  return [...(hasLocalDaemon() ? [LOCAL_HOST.id] : []), ...paired.map((h) => h.id)]
}

/**
 * The one host the sidebar shows; none while there is only one to show. The active project's
 * host wins, so a project opened by any other route is never hidden — except over a chosen host
 * with no project, which nothing could follow. With no usable choice, the active project's host.
 */
export function shownHostOf(
  filter: string,
  hostIds: readonly string[],
  projects: readonly Project[],
  activeProjectId: string | undefined,
): string | undefined {
  if (hostIds.length < 2) return undefined
  const active = projects.find((p) => p.id === activeProjectId)?.hostId
  const activeHost = active !== undefined && hostIds.includes(active) ? active : undefined
  if (!hostIds.includes(filter)) return activeHost ?? hostIds[0]
  const followsActive = activeHost !== undefined && projects.some((p) => p.hostId === filter)
  return followsActive ? activeHost : filter
}

export function projectsOnHost(projects: Project[], shown: string | undefined): Project[] {
  return shown === undefined ? projects : projects.filter((p) => p.hostId === shown)
}

/** Where choosing `hostId` lands: its project with the latest activity, the first on a tie. */
export function landingProject(projects: readonly Project[], hostId: string): Project | undefined {
  const time = (p: Project) => new Date(p.lastActivity).getTime() || 0
  return projects
    .filter((p) => p.hostId === hostId)
    .reduce<Project | undefined>((best, p) => (best && time(best) >= time(p) ? best : p), undefined)
}
