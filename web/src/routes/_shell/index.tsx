import { createFileRoute, redirect } from '@tanstack/react-router'
import { fetchProjects } from '@/lib/api'
import { useProjectStore } from '@/lib/store/projects'

export const Route = createFileRoute('/_shell/')({
  beforeLoad: async () => {
    const projects = await fetchProjects()
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
