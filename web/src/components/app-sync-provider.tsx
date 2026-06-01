import { useEffect, type ReactNode } from 'react'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { useProjectDataStore } from '@/lib/store/projects'

export function AppSyncProvider({ children }: { children: ReactNode }) {
  useEffect(() => {
    void useWorkspaceListStore.getState().fetch()
    void useProjectDataStore.getState().fetch()
    const unsubscribes = [
      useWorkspaceListStore.getState().startSync(),
      useProjectDataStore.getState().startSync(),
    ]
    return () => unsubscribes.forEach((u) => u())
  }, [])
  return <>{children}</>
}
