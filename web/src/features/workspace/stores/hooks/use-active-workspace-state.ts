import { useState, useEffect } from 'react'
import {
  getActiveWorkspaceStoreRef,
  onActiveWorkspaceStoreChange,
} from '../workspace-store-ref'
import type { WorkspaceStore } from '../workspace-store'
import type { WorkspaceState } from '../workspace-store.types'

/**
 * Safe alternative to useWorkspaceStoreContext for components that render
 * outside WorkspaceStoreContext.Provider (sidebar, global overlays, etc.).
 *
 * Returns `fallback` when no workspace is active, and automatically
 * re-subscribes when the active workspace changes.
 *
 * Pass a stable selector (module-level function or useCallback) to avoid
 * stale closure issues — the selector is captured at mount time.
 */
export function useActiveWorkspaceState<T>(
  selector: (state: WorkspaceState) => T,
  fallback: T,
): T {
  const [value, setValue] = useState<T>(() => {
    const store = getActiveWorkspaceStoreRef()
    return store ? selector(store.getState()) : fallback
  })

  useEffect(() => {
    let storeUnsub: (() => void) | null = null

    const unsub = onActiveWorkspaceStoreChange((store: WorkspaceStore | null) => {
      storeUnsub?.()
      storeUnsub = null

      if (!store) {
        setValue(fallback)
        return
      }

      setValue(selector(store.getState()))
      storeUnsub = store.subscribe((state) => {
        setValue(selector(state))
      })
    })

    return () => {
      unsub()
      storeUnsub?.()
    }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  return value
}
