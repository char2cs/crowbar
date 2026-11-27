import { qualify } from '@/lib/host-path'
import { hostIdForChat, hostIdForWorkspace } from '@/lib/host-resolve'
import { getOwningChatId, getWorkspaceScope } from '@/lib/workspace-scope'

export class OwningChatNotRecordedError extends Error {
  constructor(readonly wsId: string) {
    super(`no owning chat recorded for workspace ${wsId}`)
    this.name = 'OwningChatNotRecordedError'
  }
}

export function chatBase(chatId: string): string {
  return chatBaseOn(hostIdForChat(chatId), chatId)
}

export function chatBaseOn(hostId: string, chatId: string): string {
  return qualify(hostId, chatPath(chatId))
}

function chatPath(chatId: string): string {
  return `/v0/chats/${encodeURIComponent(chatId)}`
}

function chatAddress(wsId: string): { hostId: string; chatId: string } {
  const hostId = hostIdForWorkspace(wsId)
  const chatId = getOwningChatId(wsId)
  if (!chatId) throw new OwningChatNotRecordedError(wsId)
  return { hostId, chatId }
}

function ownedChatBase(wsId: string, leaf: string): string {
  const { hostId, chatId } = chatAddress(wsId)
  return qualify(hostId, `${chatPath(chatId)}/${leaf}`)
}

export function chatTerminalsBaseForWorkspace(wsId: string, chatId: string): string {
  return qualify(hostIdForWorkspace(wsId), `${chatPath(chatId)}/terminals`)
}

export function terminalsBaseForWorkspace(wsId: string): string {
  if (isHomeWorkspace(wsId)) return `${workspaceBase(wsId)}/terminals`
  return ownedChatBase(wsId, 'terminals')
}

export function lspBaseForWorkspace(wsId: string): string {
  return ownedChatBase(wsId, 'lsp')
}

export function gitBaseForWorkspace(wsId: string): string {
  return ownedChatBase(wsId, 'git')
}

export function reviewBaseForWorkspace(wsId: string): string {
  return ownedChatBase(wsId, 'review')
}

export function identityBaseForWorkspace(wsId: string): string {
  return ownedChatBase(wsId, 'identity')
}

export function filesBaseForWorkspace(wsId: string): string {
  if (isHomeWorkspace(wsId)) return `${workspaceBase(wsId)}/files`
  return ownedChatBase(wsId, 'files')
}

export function repoChatsBaseForWorkspace(wsId: string): string {
  const scope = getWorkspaceScope(wsId)
  if (!scope || !scope.repoId) return `${workspaceBase(wsId)}/chats`
  const p = encodeURIComponent(scope.projectId)
  const r = encodeURIComponent(scope.repoId)
  return qualify(hostIdForWorkspace(wsId), `/v0/projects/${p}/repos/${r}/chats`)
}

export function worktreeVerbBaseForWorkspace(wsId: string): string {
  const { chatId } = chatAddress(wsId)
  return `${repoChatsBaseForWorkspace(wsId)}/${encodeURIComponent(chatId)}`
}

export function workspaceBase(wsId: string): string {
  const hostId = hostIdForWorkspace(wsId)
  const scope = getWorkspaceScope(wsId)
  if (!scope) throw new Error(`no project/repo scope recorded for workspace ${wsId}`)
  const p = encodeURIComponent(scope.projectId)
  if (!scope.repoId) return qualify(hostId, `/v0/projects/${p}/home`)
  const r = encodeURIComponent(scope.repoId)
  const w = encodeURIComponent(wsId)
  return qualify(hostId, `/v0/projects/${p}/repos/${r}/workspaces/${w}`)
}

export function isHomeWorkspace(wsId: string): boolean {
  return getWorkspaceScope(wsId)?.repoId === ''
}
