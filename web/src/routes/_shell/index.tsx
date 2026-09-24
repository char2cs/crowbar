import { createFileRoute, redirect } from '@tanstack/react-router'
import { dataOf } from '@/lib/loadable'
import { useProjectDataStore, useProjectStore } from '@/lib/store/projects'

export const Route = createFileRoute('/_shell/')({
  beforeLoad: async () => {
    // Through the project loadable, not a raw GET: boot also asks it (the
    // background hydrate and the sync engine), and every ask made before the
    // request goes out shares that one request.
    await useProjectDataStore.getState().fetch()
    const data = useProjectDataStore.getState().data
    const known = dataOf(data)
    if (data.status === 'error' && !known) throw data.error
    const projects = known ?? []
    if (projects.length === 0) {
      throw redirect({ to: '/oobe' })
    }

    const activeId = useProjectStore.getState().activeProjectId
    const project = projects.find((p) => p.id === activeId) ?? projects[0]

    throw redirect({
      to: '/ide/$projectId/home',
      params: { projectId: project.id },
    })
  },
})
