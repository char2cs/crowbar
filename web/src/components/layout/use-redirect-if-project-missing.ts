import { useEffect } from 'react'
import type { NavigateFn } from '@tanstack/react-router'
import { selectProjectsSynced, useSelect } from '@/core/entities/select'
import type { Project } from '@/lib/types'

/**
 * Leaves a route whose project is gone, whatever took it: `allProjects` follows the
 * projects scope, and `/` redirects to a project that exists or to `/oobe`. Gated on
 * the scope's first snapshot, before which an absent project is unknown, not gone.
 */
export function useRedirectIfProjectMissing(
  activeProjectIdFromRoute: string | undefined,
  allProjects: Project[],
  navigate: NavigateFn,
): void {
  const synced = useSelect(selectProjectsSynced)
  useEffect(() => {
    if (!synced) return
    if (!activeProjectIdFromRoute) return
    if (allProjects.some((p) => p.id === activeProjectIdFromRoute)) return
    void navigate({ to: '/' })
  }, [synced, activeProjectIdFromRoute, allProjects, navigate])
}
