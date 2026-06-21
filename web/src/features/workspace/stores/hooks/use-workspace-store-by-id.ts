import { useStore } from 'zustand'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import type { WorkspaceState } from '@/features/workspace/stores/workspace-store.types'

/**
 * Reactively reads a slice of any workspace store by ID, without requiring a
 * WorkspaceStoreContext provider. Safe to call from global surfaces (e.g. the
 * git sidebar) where no per-workspace context is mounted.
 *
 * The registry returns a vanilla zustand store (a StoreApi object, not a
 * callable hook), so subscription goes through zustand's `useStore`.
 */
export function useWorkspaceStoreById<T>(wsId: string, selector: (s: WorkspaceState) => T): T {
  return useStore(getOrCreateWorkspaceStore(wsId), selector)
}
