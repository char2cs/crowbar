import { Navigate } from '@tanstack/react-router'
import { useProjectStore } from '@/core/entities/active-project'
import { selectProjects, selectProjectsSynced, useSelect } from '@/core/entities/select'
import type { Project } from '@/lib/types'

type Landing = { to: '/oobe' } | { to: '/ide/$projectId/home'; params: { projectId: string } }

/** Where the app opens onto, given the project list. */
export function landingFor(projects: Project[], activeId: string): Landing {
  const project = projects.find((p) => p.id === activeId) ?? projects[0]
  if (!project) return { to: '/oobe' }
  return { to: '/ide/$projectId/home', params: { projectId: project.id } }
}

/** The landing until the projects scope's first snapshot; the stream asks for it again
 *  whenever the daemon serves, so there is nothing to retry. */
export function ShellLanding() {
  const synced = useSelect(selectProjectsSynced)
  const projects = useSelect(selectProjects)
  const activeId = useProjectStore((s) => s.activeProjectId)
  if (!synced) return null
  return <Navigate {...landingFor(projects, activeId)} replace />
}
