import { useEffect, useState } from 'react'
import { Navigate, useNavigate } from '@tanstack/react-router'
import { InlineError } from '@/components/ui/inline-error'
import { LoadingSpinner } from '@/components/ui/loading-spinner'
import { useProjectStore } from '@/core/entities/active-project'
import { selectProjects, selectProjectsSynced, useSelect } from '@/core/entities/select'
import { resubscribe, watchScope } from '@/core/stream/stream'
import { useConnectionStore } from '@/lib/ws/connection-store'
import { HostProjectsPending } from '@/features/hosts/components/host-projects-pending'
import { hasLocalDaemon } from '@/features/hosts/lib/local-daemon'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import type { Project } from '@/lib/types'

type Landing =
  | { to: '/oobe' }
  | { to: '/add-host' }
  | { to: '/ide/$projectId/home'; params: { projectId: string } }

/** Where the app opens onto, given the project list; null while a paired host may still list
 *  one. The phone app has no daemon of its own, so it has no first run either. */
export function landingFor(projects: Project[], activeId: string): Landing | null {
  const project = projects.find((p) => p.id === activeId) ?? projects[0]
  if (project) return { to: '/ide/$projectId/home', params: { projectId: project.id } }
  if (hasLocalDaemon()) return { to: '/oobe' }
  return useHostConfigStore.getState().hosts.length === 0 ? { to: '/add-host' } : null
}

/** Whether the landing waits on the projects scope: only an install with a daemon of its own. */
export function awaitsProjectsScope(synced: boolean): boolean {
  return !synced && hasLocalDaemon()
}

// One failed snapshot is a daemon restarting; a second in a row is a read that fails.
const PERSISTENT = 2

const NOT_LOADED = new Error('The daemon closed the stream before it sent your projects.')

/** The landing until the projects scope's first snapshot. The stream asks for it again
 *  whenever the daemon serves; a migrating daemon shows its progress, and a snapshot
 *  that keeps failing offers Retry. */
export function ShellLanding() {
  const synced = useSelect(selectProjectsSynced)
  const projects = useSelect(selectProjects)
  const activeId = useProjectStore((s) => s.activeProjectId)
  const migrating = useConnectionStore((s) => s.migrating)
  const [failures, setFailures] = useState(0)
  const navigate = useNavigate()
  useEffect(() => watchScope('projects', { failed: setFailures }), [])
  if (!awaitsProjectsScope(synced)) {
    const landing = landingFor(projects, activeId)
    if (landing) return <Navigate {...landing} replace />
    return (
      <HostProjectsPending
        onProject={(projectId) =>
          void navigate({ to: '/ide/$projectId/home', params: { projectId }, replace: true })
        }
        onAddHost={() => void navigate({ to: '/add-host' })}
      />
    )
  }
  // A migrating daemon refuses every read; that is progress, not a failure.
  if (migrating) {
    return (
      <div className="flex flex-1 items-center justify-center p-8">
        <LoadingSpinner label={migrating.message} showLabel />
      </div>
    )
  }
  if (failures < PERSISTENT) return null
  const retry = () => {
    setFailures(0)
    resubscribe('projects')
  }
  return <InlineError error={NOT_LOADED} onRetry={retry} title="Could not load your projects" />
}
