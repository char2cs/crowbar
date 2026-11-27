import { useSyncExternalStore } from 'react'
import { useStore } from 'zustand'
import {
  detachedWorkspaceStore,
  getWorkspaceStore,
  subscribeWorkspaceRegistry,
} from '@/features/workspace/stores/workspace-store-registry'
import type { WorkspaceStore } from '@/features/workspace/stores/workspace-store'
import type { WorkspaceState } from '@/features/workspace/stores/workspace-store.types'

export function useRegisteredWorkspaceStore(wsId: string): WorkspaceStore {
  return useSyncExternalStore(
    subscribeWorkspaceRegistry,
    () => getWorkspaceStore(wsId) ?? detachedWorkspaceStore(),
  )
}

export function useWorkspaceStoreById<T>(wsId: string, selector: (s: WorkspaceState) => T): T {
  return useStore(useRegisteredWorkspaceStore(wsId), selector)
}
