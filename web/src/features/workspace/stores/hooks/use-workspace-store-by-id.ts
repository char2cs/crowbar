import { useSyncExternalStore } from 'react'
import { useStore } from 'zustand'
import {
  detachedWorkspaceStore,
  getWorkspaceStore,
  subscribeWorkspaceRegistry,
} from '@/features/workspace/stores/workspace-store-registry'
import type { WorkspaceStore } from '@/features/workspace/stores/workspace-store'
import type { WorkspaceState } from '@/features/workspace/stores/workspace-store.types'

/**
 * `wsId`'s store if `WorkspaceHost` has it mounted, else an empty detached
 * one — re-read when the registry changes. Rendering never creates a store
 * (C6): a surface that merely shows a workspace's data must not bring its
 * whole store into existence.
 */
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
