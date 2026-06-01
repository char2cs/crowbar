import { useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { hydratePreferences, hydrateSidebar } from '@/lib/persistence/hydrate'
import { apiFetch } from '@/lib/api'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useProjectStore } from '@/lib/store/projects'
import type { Repo } from '@/lib/store/sidebar'
import type { Project } from '@/lib/types'

interface HydrationGateProps {
  children: ReactNode
}

export function HydrationGate({ children }: HydrationGateProps) {
  const [hydrated, setHydrated] = useState(false)

  useEffect(() => {
    async function boot() {
      // Step 1+2: seed stores from API — must happen BEFORE hydrateSidebar
      // because hydrateSidebar maps over s.repos to apply hierarchy overrides
      await Promise.all([
        apiFetch<Repo[]>('/api/v0/workspaces')
          .then(repos => useSidebarStore.getState().setRepos(repos))
          .catch(() => {}),
        apiFetch<Project[]>('/api/v0/projects')
          .then(projects => useProjectStore.getState().setProjects(projects))
          .catch(() => {}),
      ])

      // Step 3+4: overlay IDB-persisted state on top of API data
      await Promise.all([hydratePreferences(), hydrateSidebar()])
    }

    boot()
      .catch(() => {})
      .finally(() => setHydrated(true))
  }, [])

  if (!hydrated) return null

  return <>{children}</>
}
