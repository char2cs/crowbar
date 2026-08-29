import { rowsFromRepo } from './rows-from-repo'
import type { Repo } from '@/lib/store/sidebar'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

/**
 * One project's rows — spec §4: "nothing on screen belongs to a project
 * other than the one you are in." A repo with no `projectId` yet (the
 * backend still catching up) belongs to no space and renders nowhere until
 * it arrives, same as it already did in the flat, all-repos tree.
 */
export function rowsForProject(repos: readonly Repo[], projectId: string): SidebarRow[] {
  return repos.filter((r) => r.projectId === projectId).flatMap(rowsFromRepo)
}
