import { createFileRoute, redirect, useNavigate } from '@tanstack/react-router'
import { dataOf } from '@/lib/loadable'
import { useProjectDataStore, useProjectStore } from '@/lib/store/projects'
import { hasLocalDaemon } from '@/features/hosts/lib/local-daemon'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { HostProjectsPending } from '@/features/hosts/components/host-projects-pending'

export const Route = createFileRoute('/_shell/')({
  beforeLoad: async () => {
    // Through the project loadable, not a raw GET: the sync engine joins this
    // read, and fetch() settles with the newest answer if one supersedes it.
    await useProjectDataStore.getState().fetch()
    const data = useProjectDataStore.getState().data
    const known = dataOf(data)
    if (data.status === 'error' && !known) throw data.error
    const projects = known ?? []
    if (projects.length === 0) {
      if (hasLocalDaemon()) throw redirect({ to: '/oobe' })
      if (useHostConfigStore.getState().hosts.length === 0) throw redirect({ to: '/add-host' })
      // A paired host lists its projects once it is up: the landing waits for them.
      return
    }

    const activeId = useProjectStore.getState().activeProjectId
    const project = projects.find((p) => p.id === activeId) ?? projects[0]

    throw redirect({
      to: '/ide/$projectId/home',
      params: { projectId: project.id },
    })
  },
  component: AwaitingHostProjects,
})

function AwaitingHostProjects() {
  const navigate = useNavigate()
  return (
    <HostProjectsPending
      onProject={(projectId) =>
        void navigate({ to: '/ide/$projectId/home', params: { projectId }, replace: true })
      }
      onAddHost={() => void navigate({ to: '/add-host' })}
    />
  )
}
