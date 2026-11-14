import { qualify } from '@/lib/host-path'
import { hostIdForChat, hostIdForWorkspace } from '@/lib/host-resolve'
import { getOwningChatId, getWorkspaceScope } from '@/lib/workspace-scope'

// Builders hand `qualify` bare `/v0/...` paths (desktop's API_BASE is `crowbar://localhost`), and
// resolve the host before the chat so an unplaceable workspace never reads as "chat not loaded".

// Thrown by the chat-addressed builders when `wsId` has no recorded owning chat: a recording gap,
// never a 404 to guess at. The daemon mints owners on first read, so callers can say "still
// loading" (chatNotLoadedYet) rather than surface this message.
export class OwningChatNotRecordedError extends Error {
  constructor(readonly wsId: string) {
    super(`no owning chat recorded for workspace ${wsId}`)
    this.name = 'OwningChatNotRecordedError'
  }
}

// The flat chat-scoped base `/v0/chats/:chatId`: chat ids are globally unique (spec §7.1) and the
// daemon resolves the worktree behind the chat. The host is that of the workspace recording
// `chatId` as owner; a chat no recorded scope owns (a sibling) throws.
export function chatBase(chatId: string): string {
  return chatBaseOn(hostIdForChat(chatId), chatId)
}

/** {@link chatBase} for a caller that already knows the chat's host. */
export function chatBaseOn(hostId: string, chatId: string): string {
  return qualify(hostId, chatPath(chatId))
}

function chatPath(chatId: string): string {
  return `/v0/chats/${encodeURIComponent(chatId)}`
}

/** The host and owning chat of `wsId`, or the error saying which is missing. */
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

// The terminals base for a specific chat opened in `wsId` (an agent pane's own PTY); a sibling
// chat is no workspace's recorded owner, so only `wsId` can place it.
export function chatTerminalsBaseForWorkspace(wsId: string, chatId: string): string {
  return qualify(hostIdForWorkspace(wsId), `${chatPath(chatId)}/terminals`)
}

// Terminals for `wsId`'s owning chat: sessions belong to a chat, since sibling chats share a
// worktree but never each other's shells. The project home keeps its `/home/terminals` mount so a
// home shell never waits on an owner; every other workspace throws without a recorded owner.
export function terminalsBaseForWorkspace(wsId: string): string {
  if (isHomeWorkspace(wsId)) return `${workspaceBase(wsId)}/terminals`
  return ownedChatBase(wsId, 'terminals')
}

// Editor/LSP for `wsId`'s owning chat. Owned like terminals: the daemon keys the LSP session by
// the chat id, so a sibling opens its own. The project home has no LSP surface — callers check
// isHomeWorkspace and skip it (as lsp-client.ts does) rather than catch the throw.
export function lspBaseForWorkspace(wsId: string): string {
  return ownedChatBase(wsId, 'lsp')
}

// Git for `wsId`'s owning chat. Shared state — every chat on the worktree sees the same status
// and one fanned-out push; it goes through a chat only because a route may name nothing else
// (spec law 1). Throws without a recorded owner; workspace-scoped routes are not a fallback.
export function gitBaseForWorkspace(wsId: string): string {
  return ownedChatBase(wsId, 'git')
}

// Review (branch-review read, windowed diff, merge strategy) for `wsId`'s owning chat: shared
// state with the same addressing and throw contract as gitBaseForWorkspace.
export function reviewBaseForWorkspace(wsId: string): string {
  return ownedChatBase(wsId, 'review')
}

// The human's GitHub/git identity for `wsId`'s remote: shared state with the same addressing and
// throw contract as gitBaseForWorkspace.
export function identityBaseForWorkspace(wsId: string): string {
  return ownedChatBase(wsId, 'identity')
}

// Files (REST and the file-change stream) for `wsId`: shared state like git. The project home has
// files but no worktree or chat, so it keeps its own `/home/files` surface; every other workspace
// goes through its owning chat and throws without one.
export function filesBaseForWorkspace(wsId: string): string {
  if (isHomeWorkspace(wsId)) return `${workspaceBase(wsId)}/files`
  return ownedChatBase(wsId, 'files')
}

// The repo-scoped chats collection `/v0/projects/:p/repos/:r/chats`: chat lifecycle keeps its
// nesting (spec §7.1), unlike chatBase. The project home has no repo, so it falls back to
// workspaceBase's `/home` mount, which also throws for an unrecorded scope.
export function repoChatsBaseForWorkspace(wsId: string): string {
  const scope = getWorkspaceScope(wsId)
  if (!scope || !scope.repoId) return `${workspaceBase(wsId)}/chats`
  const p = encodeURIComponent(scope.projectId)
  const r = encodeURIComponent(scope.repoId)
  return qualify(hostIdForWorkspace(wsId), `/v0/projects/${p}/repos/${r}/chats`)
}

// Base for the seven worktree lifecycle verbs (lock, sync, merge-into-parent, reparent, rebase,
// retry-provision, detach-holder) on the owning chat, beside the chat's other repo-scoped verbs
// (spec §4.3). Throws without a recorded owner; the project home has none of these verbs.
export function worktreeVerbBaseForWorkspace(wsId: string): string {
  const { chatId } = chatAddress(wsId)
  return `${repoChatsBaseForWorkspace(wsId)}/${encodeURIComponent(chatId)}`
}

// The hierarchical base `/v0/projects/:p/repos/:r/workspaces/:w` (home: `/v0/projects/:p/home`),
// resolved from the route-recorded scope; throws when it's unknown rather than 404. A branch's
// own sidebar placement uses it: position is a fact about the workspace, not a chat.
export function workspaceBase(wsId: string): string {
  const hostId = hostIdForWorkspace(wsId)
  const scope = getWorkspaceScope(wsId)
  if (!scope) throw new Error(`no project/repo scope recorded for workspace ${wsId}`)
  const p = encodeURIComponent(scope.projectId)
  // Home workspaces have no repoId — they route to /v0/projects/:p/home/...
  if (!scope.repoId) return qualify(hostId, `/v0/projects/${p}/home`)
  const r = encodeURIComponent(scope.repoId)
  const w = encodeURIComponent(wsId)
  return qualify(hostId, `/v0/projects/${p}/repos/${r}/workspaces/${w}`)
}

// A home workspace's scope has an empty repoId. Home has no git surface, so callers skip git
// data and streams (files and threads stay). False for an unknown scope, so others keep theirs.
export function isHomeWorkspace(wsId: string): boolean {
  return getWorkspaceScope(wsId)?.repoId === ''
}
