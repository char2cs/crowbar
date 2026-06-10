import { useCallback } from 'react'
import type { StoreApi, UseBoundStore } from 'zustand'

interface HasFetch<K extends unknown[]> {
  fetch: (...args: K) => unknown
}

export function useRetry<S extends HasFetch<K>, K extends unknown[]>(
  useStore: UseBoundStore<StoreApi<S>>,
  ...args: K
): () => void {
  return useCallback(() => {
    void useStore.getState().fetch(...args)
  }, [useStore, ...args])
}
