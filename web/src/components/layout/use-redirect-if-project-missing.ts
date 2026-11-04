import { useCallback, useEffect, useSyncExternalStore } from 'react'
import type { NavigateFn } from '@tanstack/react-router'
import { subscribeHostResolution } from '@/lib/host-resolve'
import { isProjectGone } from '@/lib/host-resolve'

/**
 * Leaves for `/` (its own router picks a project that exists or `/oobe`) once the routed
 * project is gone, whatever took it (deleted, its host revoked). Before the project list
 * is synced and every paired host has answered, an absent project is unknown, not gone.
 */
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
