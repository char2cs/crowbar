import { readVisibleRepoTree } from '@/lib/store/project-visibility'
import { useSidebarStore } from '@/lib/store/sidebar'

/**
 * Rebuild the sidebar repo tree directly from the entity cache and push it into
 * the sidebar store. Unlike the workspace-list loadable-slice fetch (which
 * collapses bursts and may return an in-flight stale read), this is an immediate
 * un-collapsed read: callers that just wrote a DTO to the cache use it to
 * guarantee the new repo/workspace is in the sidebar BEFORE they navigate into
 * it, so the /ide route guard doesn't treat it as unknown and bounce.
 *
 * Scoped to the VISIBLE projects (active + expanded), like every other read of
 * the cross-project entity cache — see lib/store/project-visibility.ts.
 */
export async function syncSidebarFromCache(): Promise<void> {
  useSidebarStore.getState().setRepos(await readVisibleRepoTree())
}
