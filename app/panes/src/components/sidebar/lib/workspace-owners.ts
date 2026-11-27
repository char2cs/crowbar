import type { Chat, Workspace } from '@/core/entities/repo-types'

export function resolveHomeOwnerId(
  homeId: string,
  defaultOwningChatId: string | undefined,
  chats: readonly Chat[],
): string {
  if (defaultOwningChatId) return defaultOwningChatId
  const owner = chats.find((c) => c.ownsWorktree && c.workspaceId === homeId)
  return owner?.id ?? homeId
}

export function resolveOwnerChats(workspaces: readonly Workspace[]): Map<string, string> {
  const ownerChats = new Map<string, string>()
  for (const ws of workspaces) {
    if (ws.owningChatId) ownerChats.set(ws.id, ws.owningChatId)
  }
  return ownerChats
}

export function resolveOwnerOfChat(
  ownerChats: ReadonlyMap<string, string>,
  chats: readonly Chat[],
): Map<string, string> {
  const ownerOfChat = new Map<string, string>()
  for (const [wsId, chatId] of ownerChats) ownerOfChat.set(chatId, wsId)
  for (const chat of chats) {
    if (!chat.ownsWorktree || !chat.workspaceId || ownerChats.has(chat.workspaceId)) continue
    ownerOfChat.set(chat.id, chat.workspaceId)
  }
  return ownerOfChat
}
