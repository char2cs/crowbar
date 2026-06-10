// web/src/features/workspace/stores/workspace-context.ts
import { createContext, useContext } from 'react'
import { useStore } from 'zustand'
import type { WorkspaceStore } from './workspace-store'
import type { WorkspaceState } from './workspace-store.types'

export const WorkspaceStoreContext = createContext<WorkspaceStore | null>(null)

/**
 * Returns a value from the active workspace store using a selector.
 * Must be called inside a component tree wrapped with WorkspaceStoreContext.Provider.
 */
export function useWorkspaceStoreContext<T>(selector: (state: WorkspaceState) => T): T {
  const store = useContext(WorkspaceStoreContext)
  if (!store)
    throw new Error('useWorkspaceStoreContext must be used inside WorkspaceStoreContext.Provider')
  return useStore(store, selector)
}

/**
 * Returns the raw WorkspaceStore instance (for imperative access outside React).
 */
export function useWorkspaceStore(): WorkspaceStore {
  const store = useContext(WorkspaceStoreContext)
  if (!store)
    throw new Error('useWorkspaceStore must be used inside WorkspaceStoreContext.Provider')
  return store
}
