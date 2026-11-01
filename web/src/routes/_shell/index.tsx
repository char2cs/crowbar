import { createFileRoute, redirect } from '@tanstack/react-router'
import { landingFor, ShellLanding } from '@/components/layout/shell-landing'
import { dataOf } from '@/lib/loadable'
import { useProjectDataStore, useProjectStore } from '@/lib/store/projects'

export const Route = createFileRoute('/_shell/')({
  beforeLoad: async () => {
    // Through the project loadable, not a raw GET: the sync engine joins this
    // read, and fetch() settles with the newest answer if one supersedes it.
    await useProjectDataStore.getState().fetch()
    const known = dataOf(useProjectDataStore.getState().data)
    if (known) throw redirect(landingFor(known, useProjectStore.getState().activeProjectId))
  },
  component: ShellLanding,
})
