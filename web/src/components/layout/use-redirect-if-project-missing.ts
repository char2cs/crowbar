import { useEffect } from 'react'
import type { NavigateFn } from '@tanstack/react-router'
import { useProjectDataStore } from '@/lib/store/projects'
import type { Project } from '@/lib/types'

/**
 * The one invariant that makes "viewing a project that no longer exists"
 * unreachable, regardless of what took it: this client's own space delete, a
 * teammate's, anything — `allProjects` is kept live by the `/v0/projects` WS
 * stream (projects.ts's own §6 note), so a tombstone lands here the same
 * render pass it lands anywhere else. `removal-commit.ts`'s `leaveIfRemoved`
 * tries to pick a good target BEFORE its own delete request fires, but its
 * `hiddenIds` check only ever matches a 'workspace'/'folder'/'chat' removal —
 * a 'repo'/'project' hold's `hiddenIds` names repo/project ids, never a
 * workspace id, so it silently does nothing for exactly the two removals that
 * can strand the user on project home with nothing left to render (caught
 * live: deleting the space you're sitting on left "Project Home unavailable"
 * on screen, with the now-gone project's sidebar still drawn behind it). `/`
 * is `_shell/index.tsx`'s own router: it already redirects to a project that
 * DOES exist, or to `/oobe` once none are left — the legitimate "no more
 * views" default this falls back to.
 *
 * Gated on `status === 'success'`: a `staleData` snapshot mid-refetch, or the
 * store's own idle instant before its first fetch resolves, must never read
 * as "this project is gone" and bounce a perfectly live route.
 */
export function useRedirectIfProjectMissing(
  activeProjectIdFromRoute: string | undefined,
  allProjects: Project[],
  navigate: NavigateFn,
): void {
  const projectsStatus = useProjectDataStore((s) => s.data.status)
  useEffect(() => {
    if (projectsStatus !== 'success') return
    if (!activeProjectIdFromRoute) return
    if (allProjects.some((p) => p.id === activeProjectIdFromRoute)) return
    void navigate({ to: '/' })
  }, [projectsStatus, activeProjectIdFromRoute, allProjects, navigate])
}
