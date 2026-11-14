import { isWorkspaceLockedInSidebar } from '@/core/entities/repo-queries'
import { selectRepos, useSelect } from '@/core/entities/select'

/** Whether files of `wsId` are read-only: a locked workspace refuses every write, so its editors
 *  refuse typing up front instead of failing at save. */
export function useWorkspaceReadOnly(wsId: string | null | undefined): boolean {
  return useSelect((s) => isWorkspaceLockedInSidebar(selectRepos(s), wsId ?? null))
}
