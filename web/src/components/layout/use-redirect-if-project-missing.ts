import { useCallback, useEffect, useSyncExternalStore } from 'react'
import type { NavigateFn } from '@tanstack/react-router'
import { subscribeHostResolution } from '@/lib/host-resolve'
import { isProjectGone } from '@/lib/host-unavailability'
import { dataOf, type Loadable } from '@/lib/loadable'
import { useProjectDataStore } from '@/lib/store/projects'
import type { Project } from '@/lib/types'

const holds = (data: Loadable<Project[]>, projectId: string) =>
  dataOf(data)?.some((p) => p.id === projectId) === true

/**
 * Leaves for `/` (its own router picks a project that exists) when the routed project is gone:
 * at once when a loaded list that held it drops it (deleted, its host revoked), and otherwise
 * only once every paired host has said what it has — a project in no cached row may be on one.
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

  useEffect(() => {
    if (!projectId) return
    return useProjectDataStore.subscribe(({ data }, prev) => {
      const lost =
        data.status === 'success' && holds(prev.data, projectId) && !holds(data, projectId)
      // A list that says it is gone outright is left by the effect above, once.
      if (lost && !isProjectGone(projectId)) void navigate({ to: '/' })
    })
  }, [projectId, navigate])
}
