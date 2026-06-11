import { useEffect, type ReactNode } from 'react'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { useProjectDataStore } from '@/lib/store/projects'
import { useSidebarStore } from '@/lib/store/sidebar'
import { dataOf } from '@/lib/loadable'

export function AppSyncProvider({ children }: { children: ReactNode }) {
  useEffect(() => {
    void useWorkspaceListStore.getState().fetch()
    void useProjectDataStore.getState().fetch()
    const unsubscribes = [
      useWorkspaceListStore.getState().startSync(),
      useProjectDataStore.getState().startSync(),
      // The sidebar tree renders from useSidebarStore.repos, which is seeded
      // once at boot (HydrationGate). A /v0/ws/workspaces push triggers a
      // refetch into useWorkspaceListStore — propagate that fresh data into
      // the sidebar so workspaces created by other clients appear without a
      // reload (BUG-014). mergeRepos is additive, so optimistic in-app state
      // and persisted hierarchy overrides are preserved.
      useWorkspaceListStore.subscribe((state) => {
        const repos = dataOf(state.data)
        if (repos) useSidebarStore.getState().mergeRepos(repos)
      }),
    ]
    return () => unsubscribes.forEach((u) => u())
  }, [])
  return <>{children}</>
}
