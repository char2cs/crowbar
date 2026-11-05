import type { LandingChatPresentation } from '@/features/settings/lib/chat-presentation'
import { apiFetch } from '@/lib/api/fetch'
import { repoBase } from '@/lib/project-url'
import { operationHeaders } from '@/core/entities/run-operation'
import { chatBase } from '@/features/agent/api/agent-chat-reads'

// ── Writes ──────────────────────────────────────────────────────────
/** Start a chat under `parentId` (a chat, making a thread of it even through a folder; a folder;
 *  '' = root). Placement rides the create so a thread has lineage before its first CLI spawns.
 *  `surface`: the face the daemon forks ('terminal' = no api connection). */
export async function createChat(
  id: string,
  opId: string,
  wsId: string,
  provider: string,
  parentId = '',
  surface?: LandingChatPresentation,
): Promise<string> {
  const res = await apiFetch<{ id: string }>(`${chatBase(wsId)}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', ...operationHeaders(opId) },
    // The repo-scoped mount binds no :wsId, so it travels in the body; without it the chat anchors
    // to "" and its runner spawn 404s for want of a workspace to resolve a cwd from.
    body: JSON.stringify({
      id,
      provider,
      parentId,
      workspaceId: wsId,
      ...(surface && { surface }),
    }),
  })
  return res.id
}

/** Create a chat AND the workspace it owns in one request (model spec §4.1 "own worktree"), so the
 *  new row already has its conversation. Built off project+repo, not chatBase: no workspace exists
 *  yet, and `ownWorktree` only applies when none is named. `branch` '' lets the server name it. */
export async function createChatWithOwnWorktree(
  id: string,
  opId: string,
  projectId: string,
  repoId: string,
  provider: string,
  parentId = '',
  branch = '',
): Promise<{ id: string; workspaceId: string }> {
  return apiFetch<{ id: string; workspaceId: string }>(`${repoBase(projectId, repoId)}/chats`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', ...operationHeaders(opId) },
    body: JSON.stringify({
      id,
      provider,
      parentId,
      ownWorktree: true,
      ...(branch && { branch }),
    }),
  })
}
