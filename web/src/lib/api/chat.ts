import { apiFetch } from '@/lib/api'
import { formatRelativeDate } from '@/utils/date'
import type { ProjectChat } from '@/lib/store/sidebar'

/**
 * Chat as returned by the backend (domain.Chat). The backend exposes
 * `createdAt`; the sidebar's display `age` is derived on the client.
 */
export interface ChatDto {
  id: string
  wsId: string
  title: string
  parentId?: string
  status: ProjectChat['status']
  type: ProjectChat['type']
  createdAt: string
}

export function chatDtoToProjectChat(dto: ChatDto): ProjectChat {
  return {
    id: dto.id,
    wsId: dto.wsId,
    title: dto.title,
    parentId: dto.parentId,
    status: dto.status,
    type: dto.type,
    age: formatRelativeDate(dto.createdAt),
  }
}

// POST /v0/workspaces/:wsId/chats → 201, data = full chat object.
export function postChat(wsId: string, title: string): Promise<ChatDto> {
  return apiFetch(`/v0/workspaces/${encodeURIComponent(wsId)}/chats`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ title }),
  })
}

// POST /v0/chats/:id/fork → 201, data = full chat (title copied from parent).
export function forkChat(parentId: string): Promise<ChatDto> {
  return apiFetch(`/v0/chats/${encodeURIComponent(parentId)}/fork`, {
    method: 'POST',
  })
}

// PATCH /v0/chats/:id → 200, data = full chat with the new title.
export function patchChat(id: string, title: string): Promise<ChatDto> {
  return apiFetch(`/v0/chats/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ title }),
  })
}

// DELETE /v0/chats/:id → 204.
export function deleteChat(id: string): Promise<void> {
  return apiFetch(`/v0/chats/${encodeURIComponent(id)}`, { method: 'DELETE' })
}
