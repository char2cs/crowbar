import { useCallback, useMemo } from 'react'
import {
  landingProject,
  projectsOnHost,
  shownHostOf,
  sidebarHostIds,
} from '@/features/hosts/lib/host-filter'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { useSidebarStore } from '@/lib/store/sidebar'
import type { Project } from '@/lib/types'

/**
 * Which host's projects the switcher lists. A view filter only: nothing here touches a host's
 * connection, and the tree, the streams and notifications still read every project.
 */
export function useSidebarHostFilter(
  projects: Project[],
  activeProjectId: string | undefined,
  onSelectProject: (id: string) => void,
) {
  const filter = useSidebarStore((s) => s.hostFilter)
  const paired = useHostConfigStore((s) => s.hosts)
  const hostIds = useMemo(() => sidebarHostIds(paired), [paired])
  const shown = shownHostOf(filter, hostIds, projects, activeProjectId)
  const shownProjects = useMemo(() => projectsOnHost(projects, shown), [projects, shown])

  // Land on the host's latest project unless the active one is already there.
  const selectHost = useCallback(
    (hostId: string) => {
      useSidebarStore.getState().setHostFilter(hostId)
      if (projects.find((p) => p.id === activeProjectId)?.hostId === hostId) return
      const landing = landingProject(projects, hostId)
      if (landing) onSelectProject(landing.id)
    },
    [projects, activeProjectId, onSelectProject],
  )

  return { hostIds, shown, shownProjects, selectHost }
}
