import type { Chat, Workspace } from '@/core/entities/repo-types'

// The repo/project home lives outside `repo.workspaces`, so its owner comes off
// `defaultOwningChatId` (else an ownsWorktree chat). Never throws: an unresolved
// owner leaves the row under its own raw id.
export function resolveHomeOwnerId(
  homeId: string,
  defaultOwningChatId: string | undefined,
  chats: readonly Chat[],
): string {
  if (defaultOwningChatId) return defaultOwningChatId
  const owner = chats.find((c) => c.ownsWorktree && c.workspaceId === homeId)
  return owner?.id ?? homeId
}

// Workspace id -> owning chat, CONFIRMED pairs only (the workspace record is present). This feeds
// legacyTreeIndex, which folds a chat onto its workspace node; folding an unconfirmed claim would
// strip the chat with no node to fold onto. The owner need not be in `chats` yet: the DTO wins.
export function resolveOwnerChats(workspaces: readonly Workspace[]): Map<string, string> {
  const ownerChats = new Map<string, string>()
  for (const ws of workspaces) {
    if (ws.owningChatId) ownerChats.set(ws.id, ws.owningChatId)
  }
  return ownerChats
}

// Chat id -> owned workspace, present or not: confirmed pairs plus each chat's own ownsWorktree
// claim, so a row renders as a workspace while its Workspace record lags. `workspaceId` alone is
// not enough — a thread carries its parent's.
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
