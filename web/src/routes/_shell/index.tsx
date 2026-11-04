import { createFileRoute, redirect } from '@tanstack/react-router'
import { awaitsProjectsScope, landingFor, ShellLanding } from '@/components/layout/shell-landing'
import { useProjectStore } from '@/core/entities/active-project'
import { readSelect, selectProjects, selectProjectsSynced } from '@/core/entities/select'

export const Route = createFileRoute('/_shell/')({
  beforeLoad: () => {
    // Before the projects scope's first snapshot the landing waits and redirects itself.
    if (awaitsProjectsScope(readSelect(selectProjectsSynced))) return
    const landing = landingFor(
      readSelect(selectProjects),
      useProjectStore.getState().activeProjectId,
    )
    if (landing) throw redirect(landing)
  },
  component: ShellLanding,
})
