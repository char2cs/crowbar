import { createFileRoute, redirect } from '@tanstack/react-router'
import { landingFor, ShellLanding } from '@/components/layout/shell-landing'
import { useProjectStore } from '@/core/entities/active-project'
import { readSelect, selectProjects } from '@/core/entities/select'

export const Route = createFileRoute('/_shell/')({
  beforeLoad: () => {
    const landing = landingFor(
      readSelect(selectProjects),
      useProjectStore.getState().activeProjectId,
    )
    if (landing) throw redirect(landing)
  },
  component: ShellLanding,
})
