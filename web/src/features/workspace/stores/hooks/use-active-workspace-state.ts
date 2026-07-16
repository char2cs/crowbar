import { useState, useEffect, useRef } from 'react'
import { shallow } from 'zustand/shallow'
import { getActiveWorkspaceStoreRef, onActiveWorkspaceStoreChange } from '../workspace-store-ref'
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
 *
 * Every workspace-store write (pane, buffer, LSP, git, terminal mutations —
 * they all fan into one store) re-runs the selector and, by default, the
 * result is compared with `equalityFn` (zustand's `shallow` unless
 * overridden) against the previously emitted value before notifying the
 * consumer. This mirrors zustand's `useStoreWithEqualityFn` semantics: a
 * selector that returns a fresh-but-equivalent object/array on every write
 * no longer forces a re-render on every unrelated store mutation. Switching
 * the active workspace always notifies, regardless of equality, and resets
 * the comparison baseline to the new store's value.
 */
export function useActiveWorkspaceState<T>(
  selector: (state: WorkspaceState) => T,
  fallback: T,
  equalityFn: (a: T, b: T) => boolean = shallow,
): T {
  // Always-current refs so the subscription closure never goes stale.
  const selectorRef = useRef(selector)
  selectorRef.current = selector
  const fallbackRef = useRef(fallback)
  fallbackRef.current = fallback
  const equalityFnRef = useRef(equalityFn)
  equalityFnRef.current = equalityFn

  const [value, setValue] = useState<T>(() => {
    const store = getActiveWorkspaceStoreRef()
    return store ? selectorRef.current(store.getState()) : fallbackRef.current
  })
  // Tracks the last value emitted to the consumer, independent of React's
  // render cycle, so subscribe callbacks always compare against the true
  // previous value even if setValue is skipped.
  const valueRef = useRef(value)
  valueRef.current = value

  useEffect(() => {
    let storeUnsub: (() => void) | null = null

    const unsub = onActiveWorkspaceStoreChange((store: WorkspaceStore | null) => {
      storeUnsub?.()
      storeUnsub = null

      if (!store) {
        valueRef.current = fallbackRef.current
        setValue(fallbackRef.current)
        return
      }

      // A workspace switch always applies and resets the equality baseline
      // to the new store's value — it must never be skipped just because it
      // happens to be shallow-equal to the previous (different-store) value.
      const initial = selectorRef.current(store.getState())
      valueRef.current = initial
      setValue(initial)

      storeUnsub = store.subscribe((state) => {
        const next = selectorRef.current(state)
        if (equalityFnRef.current(valueRef.current, next)) return
        valueRef.current = next
        setValue(next)
      })
    })

    return () => {
      unsub()
      storeUnsub?.()
    }
  }, [])

  return value
}
