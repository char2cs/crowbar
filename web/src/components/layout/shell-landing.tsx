import { Navigate } from '@tanstack/react-router'
import { InlineError } from '@/components/ui/inline-error'
import { LoadingSpinner } from '@/components/ui/loading-spinner'
import { problemOf } from '@/lib/errors/problem'
import { dataOf } from '@/lib/loadable'
import type { Project } from '@/lib/types'
import { useProjectDataStore, useProjectStore } from '@/lib/store/projects'

type Landing = { to: '/oobe' } | { to: '/ide/$projectId/home'; params: { projectId: string } }

/** Where the app opens onto, given the project list. */
export function landingFor(projects: Project[], activeId: string): Landing {
  const project = projects.find((p) => p.id === activeId) ?? projects[0]
  if (!project) return { to: '/oobe' }
  return { to: '/ide/$projectId/home', params: { projectId: project.id } }
}

function retry(): void {
  void useProjectDataStore.getState().fetch()
}

/** The landing while there is no project list yet. It waits on the project
 *  store, which owns the read: the stream refills it once the daemon serves
 *  again, and Retry re-reads it. */
export function ShellLanding() {
  const data = useProjectDataStore((s) => s.data)
  const activeId = useProjectStore((s) => s.activeProjectId)
  const known = dataOf(data)
  if (known) return <Navigate {...landingFor(known, activeId)} replace />
  if (data.status !== 'error') return null
  // A migrating daemon refuses every read; that is progress, not a failure.
  if (problemOf(data.error).code === 'daemon_migrating') {
    return (
      <div className="flex flex-1 items-center justify-center p-8">
        <LoadingSpinner label={data.error.message} showLabel />
      </div>
    )
  }
  return <InlineError error={data.error} onRetry={retry} title="Could not load your projects" />
}
