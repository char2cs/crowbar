import { useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { hydratePreferences, hydrateSidebar, hydrateWindowPaneLayout } from '@/lib/persistence/hydrate'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useProjectStore, useProjectDataStore } from '@/lib/store/projects'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { dataOf } from '@/lib/loadable'

interface HydrationGateProps {
  children: ReactNode
}

export function HydrationGate({ children }: HydrationGateProps) {
  const [hydrated, setHydrated] = useState(false)

  useEffect(() => {
    async function boot() {
      // Fetch via LoadableSlice stores (IDB-cached, stale-while-revalidate).
      // Must complete BEFORE hydrateSidebar which applies hierarchy overrides to s.repos.
      await Promise.all([
        useWorkspaceListStore.getState().fetch(),
        useProjectDataStore.getState().fetch(),
      ])

      const repos = dataOf(useWorkspaceListStore.getState().data) ?? []
      const projects = dataOf(useProjectDataStore.getState().data) ?? []

      useSidebarStore.getState().setRepos(repos)
      useProjectStore.getState().setProjects(projects)

      // Overlay IDB-persisted UI state on top of API data. Task 26: pane/buffer
      // layout is window-level now — hydrated ONCE here, before any
      // WorkspaceView mounts, rather than once per workspace.
      await Promise.all([hydratePreferences(), hydrateSidebar(), hydrateWindowPaneLayout()])
    }

    boot()
      .catch(() => {})
      .finally(() => setHydrated(true))
  }, [])

  if (!hydrated) return null

  return <>{children}</>
}
