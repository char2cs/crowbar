import { isWorkspaceLockedInSidebar } from '@/core/entities/repo-queries'
import { selectRepos, useSelect } from '@/core/entities/select'

export function useWorkspaceReadOnly(wsId: string | null | undefined): boolean {
  return useSelect((s) => isWorkspaceLockedInSidebar(selectRepos(s), wsId ?? null))
}
