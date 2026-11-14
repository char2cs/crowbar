import { useCallback, useEffect, useSyncExternalStore } from 'react'
import type { NavigateFn } from '@tanstack/react-router'
import { subscribeHostResolution } from '@/lib/host-resolve'
import { isProjectGone } from '@/lib/host-resolve'

// Leaves for `/` once the routed project is gone (deleted, host revoked). Until the list has synced
// and every paired host answered, an absent project is unknown, not gone.
export function useRedirectIfProjectMissing(
  projectId: string | undefined,
  navigate: NavigateFn,
): void {
  const gone = useSyncExternalStore(
    useCallback((onChange: () => void) => subscribeHostResolution(undefined, onChange), []),
    () => projectId !== undefined && isProjectGone(projectId),
  )
  useEffect(() => {
    if (gone) void navigate({ to: '/' })
  }, [gone, navigate])
}
