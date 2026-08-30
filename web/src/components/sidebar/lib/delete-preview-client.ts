import { apiFetch } from '@/lib/api'

/**
 * What deleting a row's subtree would take, without taking it (backend
 * addendum spec §1). The chat count is free client-side — `removal-plan.ts`
 * already walks the tree for it — but the file count sums `git status`
 * across every workspace-owning row the subtree contains, which only the
 * backend can answer in one call.
 */
export interface DeletePreview {
  chatCount: number
  fileCount: number
}

/**
 * GET .../projects/:projectId/repos/:repoId/chats/:id/delete-preview.
 *
 * `id` may name a chat or a folder (backend addendum spec §1) — this route
 * answers "what does deleting THIS node's subtree sweep away" for either.
 *
 * Modelled on `fetchWorkspace`/`fetchWorkspaces` (`web/src/lib/api.ts`): the
 * backend spec's own route shorthand reads `/repos/:rid/chats/:id/...`, but
 * every repo-scoped chat route actually mounted in this codebase nests under
 * the project too (`agent-api.ts`'s `chatBase`), so the real path carries
 * `projectId` as well.
 */
export function fetchDeletePreview(
  projectId: string,
  repoId: string,
  id: string,
): Promise<DeletePreview> {
  return apiFetch(`/v0/projects/${projectId}/repos/${repoId}/chats/${id}/delete-preview`)
}
