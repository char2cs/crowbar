import { useSyncExternalStore } from 'react'

// Each workspace's owning project+repo (and chat), keyed by wsId. Dependency-free on purpose, so
// the URL builders resolve it without pulling in the editor/Monaco workspace-store graph.
export interface WorkspaceScope {
  projectId: string
  repoId: string
  wsId: string
  /** The chat owning this worktree, straight from WorkspaceDTO.owningChatId: what chat-scoped
   *  routes are addressed by. Optional because the route can't supply it, only the sidebar's
   *  data — hence writes merge rather than overwrite it. */
  owningChatId?: string
}

const _scopes = new Map<string, WorkspaceScope>()

// Notified whenever a workspace's scope is (re)written, so useOwningChatId and
// useWorkspaceScopeReady can wait on a scope instead of reading it once at mount.
const _scopeListeners = new Map<string, Set<() => void>>()

// Every listener runs even if one throws (a throw must not starve the hooks behind
// it); the first error is rethrown afterwards, so a single failure still surfaces.
function notifyScopeListeners(wsId: string): void {
  const listeners = _scopeListeners.get(wsId)
  if (!listeners) return
  let failure: { error: unknown } | null = null
  for (const listener of listeners) {
    try {
      listener()
    } catch (error) {
      failure ??= { error }
    }
  }
  if (failure) throw failure.error
}

// Subscribe to every change of `wsId`'s scope, any field — callers interested in one field
// re-read it themselves.
export function subscribeToWorkspaceScope(wsId: string, callback: () => void): () => void {
  let listeners = _scopeListeners.get(wsId)
  if (!listeners) {
    listeners = new Set()
    _scopeListeners.set(wsId, listeners)
  }
  listeners.add(callback)
  return () => {
    listeners!.delete(callback)
    if (listeners!.size === 0) _scopeListeners.delete(wsId)
  }
}

// Merge keeping a recorded owningChatId when the incoming scope has none: the route parser writes
// here without one, and erasing the sidebar's would break every chat-scoped URL.
function mergeScope(scope: WorkspaceScope): WorkspaceScope {
  const prev = _scopes.get(scope.wsId)
  const owningChatId = scope.owningChatId || prev?.owningChatId
  return owningChatId ? { ...scope, owningChatId } : { ...scope }
}

const sameScope = (a: WorkspaceScope | undefined, b: WorkspaceScope): boolean =>
  a !== undefined &&
  a.projectId === b.projectId &&
  a.repoId === b.repoId &&
  a.owningChatId === b.owningChatId

// A write that changes nothing notifies no one: the route's write runs during IDEShell's
// render, and a listener whose answer moved with another store must not re-render from it.
function writeScope(scope: WorkspaceScope): void {
  const next = mergeScope(scope)
  if (sameScope(_scopes.get(scope.wsId), next)) return
  _scopes.set(scope.wsId, next)
  notifyScopeListeners(scope.wsId)
}

/** Record the hierarchical scope (project+repo) for a workspace from the
 *  route. It never decides which workspace is active: that is the registry's
 *  one id, written by `WorkspaceHost`. */
export function setWorkspaceScope(scope: WorkspaceScope): void {
  writeScope(scope)
}

// Record a workspace's scope without making it active. The sidebar calls this as data arrives, so
// verbs on a workspace never navigated to (Retry/Detach… on a placeholder) can build their URLs.
export function recordWorkspaceScope(scope: WorkspaceScope): void {
  writeScope(scope)
}

// Drop a deleted workspace's scope. Everything addressed through its owning chat (LSP sessions
// above all) stops on this notification instead of reconnecting to a chat that is gone.
export function forgetWorkspaceScope(wsId: string): void {
  if (!_scopes.delete(wsId)) return
  notifyScopeListeners(wsId)
}

// The router pathname for the active workspace route. Not anchored to the start
// so it also matches the hash-history in-hash path; captures exactly the three
// /ide/:projectId/:repoId/:wsId segments and stops at the next separator.
const IDE_ROUTE = /\/ide\/([^/]+)\/([^/]+)\/([^/]+)/

// Parse and record an /ide/:projectId/:repoId/:wsId pathname's scope (null off /ide). The IDE
// shell calls this synchronously during render, since workspace panels build scoped URLs in their
// own render; recording it in an effect would make the first render throw.
export function recordWorkspaceScopeFromPath(pathname: string): WorkspaceScope | null {
  const scope = parseWorkspaceScopeFromPath(pathname)
  if (scope) setWorkspaceScope(scope)
  return scope
}

// Parse an /ide/:p/:r/:wsId pathname into its scope without recording it, for render paths that
// only read the active workspace (e.g. the context pill); the IDE shell owns the registry.
export function parseWorkspaceScopeFromPath(pathname: string): WorkspaceScope | null {
  const match = pathname.match(IDE_ROUTE)
  if (!match) return null
  return { projectId: match[1], repoId: match[2], wsId: match[3] }
}

/** Testing only — clears the in-memory scope registry between tests. */
export function __resetWorkspaceScopesForTest(): void {
  _scopes.clear()
  _scopeListeners.clear()
}

// `wsId`'s recorded scope, or null — callers throw or skip rather than build a URL missing a
// segment. No "active workspace" default: an unnamed ask lands on the wrong workspace or host.
export function getWorkspaceScope(wsId: string): WorkspaceScope | null {
  return _scopes.get(wsId) ?? null
}

// The chat owning `wsId`'s worktree, or null when the scope is unrecorded or has no owner: the
// bridge from a workspace id to the chat id routes are addressed by. Callers throw or skip on null;
// there is no workspace-scoped route to fall back to.
export function getOwningChatId(wsId: string): string | null {
  return getWorkspaceScope(wsId)?.owningChatId || null
}

// The workspace whose worktree `chatId` holds, or null. Only owning chats are recorded, so a
// sibling must be placed through the workspace it is opened in.
export function getWorkspaceIdOwnedByChat(chatId: string): string | null {
  if (!chatId) return null
  for (const scope of _scopes.values()) if (scope.owningChatId === chatId) return scope.wsId
  return null
}

// Whether `wsId`'s scope is recorded (workspaceBase throws without it), as React state.
// WorkspaceHost can mount a retained workspace before the route or sidebar records it (a cold-boot
// race). For the owning chat use useOwningChatId: scope can be recorded long before its owner.
export function useWorkspaceScopeReady(wsId: string): boolean {
  return useSyncExternalStore(
    (onChange) => subscribeToWorkspaceScope(wsId, onChange),
    () => getWorkspaceScope(wsId) !== null,
  )
}

// getOwningChatId as React state: the sidebar records it asynchronously and chat-scoped routes
// throw without it, so effects must re-run once it lands.
export function useOwningChatId(wsId: string): string | null {
  return useSyncExternalStore(
    (onChange) => subscribeToWorkspaceScope(wsId, onChange),
    () => getOwningChatId(wsId),
  )
}
