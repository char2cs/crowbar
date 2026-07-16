import { useCallback, useRef } from 'react'
import type { StoreApi, UseBoundStore } from 'zustand'

interface HasFetch<K extends unknown[]> {
  fetch: (...args: K) => unknown
}

export function useRetry<S extends HasFetch<K>, K extends unknown[]>(
  useStore: UseBoundStore<StoreApi<S>>,
  ...args: K
): () => void {
  // Keep a latest-value ref of the variadic args so the returned callback stays
  // stable (spreading `...args` into the dep array can't be statically verified
  // and re-creates the callback each render). Retry always fetches with the
  // current args.
  const argsRef = useRef(args)
  argsRef.current = args
  return useCallback(() => {
    void useStore.getState().fetch(...argsRef.current)
  }, [useStore])
}
