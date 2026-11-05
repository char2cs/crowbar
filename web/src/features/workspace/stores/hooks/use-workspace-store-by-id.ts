import { useSyncExternalStore } from 'react'
import { useStore } from 'zustand'
import {
  detachedWorkspaceStore,
  getWorkspaceStore,
  subscribeWorkspaceRegistry,
} from '@/features/workspace/stores/workspace-store-registry'
import type { WorkspaceStore } from '@/features/workspace/stores/workspace-store'
import type { WorkspaceState } from '@/features/workspace/stores/workspace-store.types'

/** `wsId`'s mounted store, else an empty detached one, re-read on registry changes. Rendering
 *  never mints: merely showing a workspace's data must not bring its whole store into being. */
export function useRegisteredWorkspaceStore(wsId: string): WorkspaceStore {
  return useSyncExternalStore(
    subscribeWorkspaceRegistry,
    () => getWorkspaceStore(wsId) ?? detachedWorkspaceStore(),
  )
}

/** A slice of `wsId`'s store, without a WorkspaceStoreContext provider. */
export function useWorkspaceStoreById<T>(wsId: string, selector: (s: WorkspaceState) => T): T {
  return useStore(useRegisteredWorkspaceStore(wsId), selector)
}
