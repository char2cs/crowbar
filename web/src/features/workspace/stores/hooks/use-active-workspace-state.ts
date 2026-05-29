import { useState, useEffect, useRef } from 'react'
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
 * Unlike useWorkspaceStoreContext, this hook always uses the latest selector
 * even if the parent re-renders with a new one — safe with inline selectors.
 */
export function useActiveWorkspaceState<T>(
  selector: (state: WorkspaceState) => T,
  fallback: T,
): T {
  // Always-current refs so the subscription closure never goes stale.
  const selectorRef = useRef(selector)
  selectorRef.current = selector
  const fallbackRef = useRef(fallback)
  fallbackRef.current = fallback

  const [value, setValue] = useState<T>(() => {
    const store = getActiveWorkspaceStoreRef()
    return store ? selectorRef.current(store.getState()) : fallbackRef.current
  })

  useEffect(() => {
    let storeUnsub: (() => void) | null = null

    const unsub = onActiveWorkspaceStoreChange((store: WorkspaceStore | null) => {
      storeUnsub?.()
      storeUnsub = null

      if (!store) {
        setValue(fallbackRef.current)
        return
      }

      setValue(selectorRef.current(store.getState()))
      storeUnsub = store.subscribe((state) => {
        setValue(selectorRef.current(state))
      })
    })

    return () => {
      unsub()
      storeUnsub?.()
    }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  return value
}
