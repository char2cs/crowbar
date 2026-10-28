import { hasLocalDaemon } from '@/features/hosts/lib/local-daemon'
import { LOCAL_HOST, type Host } from '@/features/hosts/types'
import type { Project } from '@/lib/types'

/** The filter value that shows every host's projects. */
export const ALL_HOSTS = 'all'

/** Every host the sidebar can filter to: this machine's daemon, if it has one, then each paired host. */
export function sidebarHostIds(paired: readonly Host[]): string[] {
  return [...(hasLocalDaemon() ? [LOCAL_HOST.id] : []), ...paired.map((h) => h.id)]
}

/**
 * The host the sidebar shows: the saved filter, except that the active project's host wins, so
 * a project opened by any other route is never hidden. A filter that can't apply (one host
 * only, or its host gone) is all; a chosen host with no project keeps the choice, since there
 * is nothing to follow there.
 */
export function shownHostOf(
  filter: string,
  hostIds: readonly string[],
  projects: readonly Project[],
  activeProjectId: string | undefined,
): string {
  if (hostIds.length < 2 || !hostIds.includes(filter)) return ALL_HOSTS
  const active = projects.find((p) => p.id === activeProjectId)
  const followsActive =
    active !== undefined &&
    active.hostId !== filter &&
    hostIds.includes(active.hostId) &&
    projects.some((p) => p.hostId === filter)
  return followsActive ? active.hostId : filter
}

export function projectsOnHost(projects: Project[], shown: string): Project[] {
  return shown === ALL_HOSTS ? projects : projects.filter((p) => p.hostId === shown)
}

/** Where choosing `hostId` lands: its project with the latest activity, the first on a tie. */
export function landingProject(projects: readonly Project[], hostId: string): Project | undefined {
  const time = (p: Project) => new Date(p.lastActivity).getTime() || 0
  return projects
    .filter((p) => p.hostId === hostId)
    .reduce<Project | undefined>((best, p) => (best && time(best) >= time(p) ? best : p), undefined)
}
