import type { LandingChatPresentation } from '@/features/settings/lib/chat-presentation'
import { apiFetch } from '@/lib/api/fetch'
import { repoBase } from '@/lib/project-url'
import { operationHeaders } from '@/core/entities/run-operation'
import { chatBase } from '@/features/agent/api/agent-chat-reads'

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

export async function createChatWithOwnWorktree(
  id: string,
  opId: string,
  projectId: string,
  repoId: string,
  provider: string,
  parentId = '',
  branch = '',
  surface?: LandingChatPresentation,
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
      ...(surface && { surface }),
    }),
  })
}
