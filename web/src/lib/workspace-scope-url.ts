import { getOwningChatId, getWorkspaceScope } from '@/lib/workspace-scope'

/**
 * Build the flat chat-scoped API/WS base: `/v0/chats/:chatId`.
 *
 * Chat ids are globally unique, so this prefix carries no project/repo nesting
 * (backend spec §7.1). The daemon's chat-scoped middleware resolves the chat to
 * the worktree behind it, which is how a terminal opened here still gets the
 * right CWD without the URL naming a workspace.
 */
export function chatBase(chatId: string): string {
  return `/v0/chats/${encodeURIComponent(chatId)}`
}

/**
 * The terminals base for the chat that owns `wsId`'s worktree.
 *
 * Terminal sessions are owned by a CHAT, not by a worktree: sibling chats
 * routinely share one worktree and must never see each other's shells. Callers
 * that already hold a chat id (an agent chat pane) should use chatBase directly
 * — this is the bridge for callers that only hold a workspace id.
 *
 * Throws when no owning chat is recorded, matching workspaceBase's
 * fail-loudly-rather-than-404 contract; there is no workspace-scoped terminal
 * route left to fall back to.
 */
export function terminalsBaseForWorkspace(wsId: string): string {
  const chatId = getOwningChatId(wsId)
  if (!chatId) throw new Error(`no owning chat recorded for workspace ${wsId}`)
  return `${chatBase(chatId)}/terminals`
}

// §3/§7: build the hierarchical base for a workspace-scoped API/WS URL. Every
// files/git/lsp/terminal route nests under the owning project+repo now; callers
// still pass only a wsId (the identifier they hold), and the project/repo are
// resolved from the route-recorded scope (see workspace-store-registry). Throws
// when the scope is unknown so a stale/mis-ordered call fails loudly instead of
// hitting a missing-segment 404.
export function workspaceBase(wsId: string): string {
  const scope = getWorkspaceScope(wsId)
  if (!scope) throw new Error(`no project/repo scope recorded for workspace ${wsId}`)
  const p = encodeURIComponent(scope.projectId)
  // Home workspaces have no repoId — they route to /v0/projects/:p/home/...
  if (!scope.repoId) return `/v0/projects/${p}/home`
  const r = encodeURIComponent(scope.repoId)
  const w = encodeURIComponent(wsId)
  return `/v0/projects/${p}/repos/${r}/workspaces/${w}`
}

// A home (project-level) workspace has no owning repo: its scope is recorded
// with an empty repoId (see the .../home route), which is how workspaceBase
// routes it to /home/... The home workspace deliberately has no git surface on
// the backend, so callers use this to skip git data/streams (files and threads
// ARE home capabilities and stay enabled). Returns false when the scope is
// unknown so non-home workspaces keep their full surface.
export function isHomeWorkspace(wsId: string): boolean {
  return getWorkspaceScope(wsId)?.repoId === ''
}
