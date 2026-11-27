import { useCallback, useEffect, useSyncExternalStore } from 'react'
import type { NavigateFn } from '@tanstack/react-router'
import { subscribeHostResolution } from '@/lib/host-resolve'
import { isProjectGone } from '@/lib/host-resolve'

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
